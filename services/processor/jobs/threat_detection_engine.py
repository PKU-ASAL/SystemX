#!/usr/bin/env python3
"""
SysArmor Threat Detection Engine
威胁检测规则引擎
基于 Falco/Sysdig 规则引擎设计
支持 Falco 风格的条件字符串解析、列表和宏，实现类型索引，短路评估，字段缓存等优化
"""

import os
import json
import logging
import re
import yaml
import uuid
import time
import fnmatch
from collections import defaultdict, deque
from datetime import datetime
from typing import Dict, List, Optional, Any, Union, Set
from abc import ABC, abstractmethod
from enum import Enum

logger = logging.getLogger(__name__)


# ============================================================================
# 匹配策略
# ============================================================================

class RuleMatchStrategy(Enum):
    """规则匹配策略"""
    ALL = "all"           # 匹配所有规则（默认）
    FIRST = "first"       # 匹配第一个规则即停止（最快）
    HIGHEST = "highest"   # 只返回最高优先级的规则


# ============================================================================
# 字段缓存
# ============================================================================

class CachedEventData:
    """带缓存的事件数据包装器
    
    避免在多个条件中重复解析相同字段
    """
    
    def __init__(self, event_data: Dict):
        self.event_data = event_data
        self._field_cache: Dict[str, Any] = {}
    
    def get_field(self, field_path: str, extractor_func) -> Any:
        if field_path in self._field_cache:
            return self._field_cache[field_path]
        
        value = extractor_func(field_path, self.event_data)
        self._field_cache[field_path] = value
        return value


# ============================================================================
# 统计管理
# ============================================================================

class RuleStats:
    """规则统计信息"""
    
    def __init__(self, rule_id: str):
        self.rule_id = rule_id
        self.matched_count = 0
        self.evaluated_count = 0
        self.total_eval_time_ms = 0.0
        self.last_matched_at: Optional[datetime] = None
    
    @property
    def match_rate(self) -> float:
        """匹配率"""
        return self.matched_count / self.evaluated_count if self.evaluated_count > 0 else 0
    
    @property
    def avg_eval_time_ms(self) -> float:
        """平均评估时间（毫秒）"""
        return self.total_eval_time_ms / self.evaluated_count if self.evaluated_count > 0 else 0


class StatsManager:
    """统计管理器"""
    
    def __init__(self):
        self.stats: Dict[str, RuleStats] = {}
        self.global_stats = {
            'total_events': 0,
            'total_alerts': 0,
            'start_time': time.time()
        }
    
    def record_evaluation(self, rule_id: str, matched: bool, eval_time_ms: float):
        """记录规则评估"""
        if rule_id not in self.stats:
            self.stats[rule_id] = RuleStats(rule_id)
        
        stats = self.stats[rule_id]
        stats.evaluated_count += 1
        stats.total_eval_time_ms += eval_time_ms
        
        if matched:
            stats.matched_count += 1
            stats.last_matched_at = datetime.utcnow()
    
    def record_event(self, alert_count: int):
        """记录事件处理"""
        self.global_stats['total_events'] += 1
        self.global_stats['total_alerts'] += alert_count
    
    def print_stats(self):
        """打印统计信息"""
        uptime = time.time() - self.global_stats['start_time']
        
        print("\n" + "="*100)
        print("规则引擎统计信息")
        print("="*100)
        print(f"运行时间: {uptime:.2f}s | "
              f"总事件: {self.global_stats['total_events']} | "
              f"总告警: {self.global_stats['total_alerts']}")
        print("-"*100)
        print(f"{'规则ID':<40s} | {'匹配':<8s} | {'评估':<8s} | {'匹配率':<8s} | {'平均耗时'}")
        print("-"*100)
        
        for rule_id, stats in sorted(
            self.stats.items(), 
            key=lambda x: x[1].matched_count, 
            reverse=True
        ):
            print(f"{rule_id:<40s} | "
                  f"{stats.matched_count:<8d} | "
                  f"{stats.evaluated_count:<8d} | "
                  f"{stats.match_rate:<8.2%} | "
                  f"{stats.avg_eval_time_ms:.3f}ms")
        
        print("="*100 + "\n")


# ============================================================================
# 限流器
# ============================================================================

class RateLimiter:
    """规则触发频率限制器"""
    
    def __init__(self, max_alerts_per_minute: int = 100):
        self.max_alerts = max_alerts_per_minute
        self.alert_counts: Dict[str, deque] = defaultdict(deque)
    
    def should_drop_alert(self, rule_id: str) -> bool:
        """判断是否应该丢弃告警"""
        now = time.time()
        
        # 清理 1 分钟前的记录
        while self.alert_counts[rule_id] and \
              self.alert_counts[rule_id][0] < now - 60:
            self.alert_counts[rule_id].popleft()
        
        # 检查是否超过限制
        if len(self.alert_counts[rule_id]) >= self.max_alerts:
            logger.warning(f"规则 {rule_id} 触发频率超限，丢弃告警")
            return True
        
        self.alert_counts[rule_id].append(now)
        return False


# ============================================================================
# AST 节点定义
# ============================================================================

class ASTNode(ABC):
    """AST 节点基类"""
    
    @abstractmethod
    def evaluate(self, event_data: Dict) -> bool:
        """评估节点"""
        pass
    
    @abstractmethod
    def __str__(self) -> str:
        """字符串表示"""
        pass


class AndNode(ASTNode):
    """逻辑与节点"""
    def __init__(self, children: List[ASTNode]):
        self.children = children
    
    def evaluate(self, event_data: Dict) -> bool:
        # 短路评估：所有子节点都必须为 true
        for child in self.children:
            if not child.evaluate(event_data):
                return False
        return True
    
    def __str__(self) -> str:
        return f"({' and '.join(str(c) for c in self.children)})"


class OrNode(ASTNode):
    """逻辑或节点"""
    def __init__(self, children: List[ASTNode]):
        self.children = children
    
    def evaluate(self, event_data: Dict) -> bool:
        # 短路评估：任一子节点为 true 即可
        for child in self.children:
            if child.evaluate(event_data):
                return True
        return False
    
    def __str__(self) -> str:
        return f"({' or '.join(str(c) for c in self.children)})"


class NotNode(ASTNode):
    """逻辑非节点"""
    def __init__(self, child: ASTNode):
        self.child = child
    
    def evaluate(self, event_data: Dict) -> bool:
        return not self.child.evaluate(event_data)
    
    def __str__(self) -> str:
        return f"not {self.child}"


class ComparisonNode(ASTNode):
    """比较节点"""
    def __init__(self, field: str, operator: str, value: Any):
        self.field = field
        self.operator = operator
        self.value = value
    
    def evaluate(self, event_data: Union[Dict, CachedEventData]) -> bool:
        # 支持缓存的事件数据
        if isinstance(event_data, CachedEventData):
            field_value = event_data.get_field(self.field, self._get_field_value)
        else:
            field_value = self._get_field_value(self.field, event_data)
        
        if field_value is None:
            logger.debug(f"字段 {self.field} 值为 None")
            return False
        
        # 执行比较
        result = self._compare(field_value, self.operator, self.value)
        logger.debug(f"比较: {self.field}='{field_value}' {self.operator} {self.value} => {result}")
        return result
    
    def _get_field_value(self, field_path: str, event_data: Dict):
        """根据字段路径获取值，支持嵌套访问和直接键名匹配"""
        try:
            # 首先尝试直接访问完整字段名（适配 sysdig 格式）
            if field_path in event_data:
                return event_data[field_path]
            
            # 如果直接访问失败，尝试分层访问
            parts = field_path.split('.')
            current = event_data
            
            for part in parts:
                if isinstance(current, dict):
                    # 处理数组索引，如 proc.aname[1]
                    if '[' in part and ']' in part:
                        field_name = part.split('[')[0]
                        index_str = part.split('[')[1].split(']')[0]
                        try:
                            index = int(index_str)
                            if field_name in current and isinstance(current[field_name], list):
                                if 0 <= index < len(current[field_name]):
                                    current = current[field_name][index]
                                else:
                                    return None
                            else:
                                return None
                        except (ValueError, IndexError):
                            return None
                    else:
                        current = current.get(part)
                else:
                    return None
                
                if current is None:
                    return None
            
            return current
        except Exception as e:
            logger.debug(f"获取字段值失败: {field_path}, {e}")
            return None
    
    def _compare(self, left: Any, op: str, right: Any) -> bool:
        """执行比较操作"""
        try:
            if op == '=' or op == '==':
                return str(left) == str(right)
            elif op == '!=' or op == '<>':
                return str(left) != str(right)
            elif op == '<':
                return left < right
            elif op == '>':
                return left > right
            elif op == '<=':
                return left <= right
            elif op == '>=':
                return left >= right
            elif op == 'contains':
                return str(right) in str(left)
            elif op == 'icontains':
                return str(right).lower() in str(left).lower()
            elif op == 'startswith':
                return str(left).startswith(str(right))
            elif op == 'endswith':
                return str(left).endswith(str(right))
            elif op == 'in':
                if isinstance(right, (list, tuple, set)):
                    result = left in right
                    return result
                return False
            elif op == 'pmatch':  # glob pattern match
                import fnmatch
                return fnmatch.fnmatch(str(left), str(right))
            elif op == 'regex':
                pattern = str(right)
                text = str(left)
                # 处理转义序列：YAML中的双反斜杠需要转换为单反斜杠
                try:
                    pattern = pattern.encode().decode('unicode_escape')
                except Exception:
                    pass  # 如果解码失败，使用原始模式
                result = re.search(pattern, text, re.IGNORECASE) is not None
                logger.debug(f"regex 操作: pattern={repr(pattern)} text={repr(text)} result={result}")
                return result
            else:
                logger.warning(f"未知操作符: {op}")
                return False
        except Exception as e:
            logger.debug(f"比较异常: {e}")
            return False
    
    def __str__(self) -> str:
        if isinstance(self.value, (list, tuple)):
            value_str = f"({', '.join(map(str, self.value))})"
        else:
            value_str = f'"{self.value}"' if isinstance(self.value, str) else str(self.value)
        return f"{self.field} {self.operator} {value_str}"


class MacroNode(ASTNode):
    """宏引用节点"""
    def __init__(self, name: str):
        self.name = name
        self.expanded = None
    
    def evaluate(self, event_data: Dict) -> bool:
        if self.expanded:
            return self.expanded.evaluate(event_data)
        raise RuntimeError(f"宏 '{self.name}' 未展开")
    
    def __str__(self) -> str:
        return f"<macro:{self.name}>"


# ============================================================================
# 词法分析器和解析器
# ============================================================================

class Token:
    """词法单元"""
    def __init__(self, type: str, value: str, pos: int):
        self.type = type
        self.value = value
        self.pos = pos
    
    def __repr__(self):
        return f"Token({self.type}, '{self.value}', {self.pos})"


class Lexer:
    """词法分析器"""
    
    # 运算符关键字
    OPERATORS = {
        '=', '==', '!=', '<>', '<', '>', '<=', '>=',
        'contains', 'icontains', 'startswith', 'endswith',
        'in', 'pmatch', 'regex'
    }
    
    LOGICAL = {'and', 'or', 'not'}
    
    def __init__(self, text: str):
        self.text = text
        self.pos = 0
        self.tokens = []
    
    def tokenize(self) -> List[Token]:
        """将输入文本分词"""
        self.tokens = []
        
        while self.pos < len(self.text):
            # 跳过空白字符
            if self.text[self.pos].isspace():
                self.pos += 1
                continue
            
            # 括号
            if self.text[self.pos] in '()':
                self.tokens.append(Token('PAREN', self.text[self.pos], self.pos))
                self.pos += 1
                continue
            
            # 逗号
            if self.text[self.pos] == ',':
                self.tokens.append(Token('COMMA', ',', self.pos))
                self.pos += 1
                continue
            
            # 字符串（单引号或双引号）
            if self.text[self.pos] in '"\'':
                string_val = self._read_string()
                self.tokens.append(Token('STRING', string_val, self.pos))
                continue
            
            # 运算符（先尝试多字符运算符）
            if self._try_operator():
                continue
            
            # 标识符或关键字
            word = self._read_word()
            if word:
                word_lower = word.lower()
                if word_lower in self.LOGICAL:
                    self.tokens.append(Token('LOGICAL', word_lower, self.pos))
                elif word_lower in self.OPERATORS:
                    self.tokens.append(Token('OPERATOR', word_lower, self.pos))
                else:
                    # 字段名或标识符
                    self.tokens.append(Token('IDENTIFIER', word, self.pos))
        
        return self.tokens
    
    def _try_operator(self) -> bool:
        """匹配多字符运算符"""
        if self.pos + 1 < len(self.text):
            two_char = self.text[self.pos:self.pos+2]
            if two_char in ['==', '!=', '<=', '>=', '<>']:
                self.tokens.append(Token('OPERATOR', two_char, self.pos))
                self.pos += 2
                return True
        
        # 单字符运算符
        if self.text[self.pos] in '=<>':
            self.tokens.append(Token('OPERATOR', self.text[self.pos], self.pos))
            self.pos += 1
            return True
        
        return False
    
    def _read_string(self) -> str:
        """读取字符串字面量，处理转义序列"""
        quote = self.text[self.pos]
        self.pos += 1
        result = []
        
        while self.pos < len(self.text) and self.text[self.pos] != quote:
            if self.text[self.pos] == '\\' and self.pos + 1 < len(self.text):
                # 处理转义字符 - 不进行任何转换，直接保留
                # 因为 YAML 已经处理过一次转义了
                result.append(self.text[self.pos])  # 添加反斜杠
                self.pos += 1
                if self.pos < len(self.text):
                    result.append(self.text[self.pos])  # 添加下一个字符
                    self.pos += 1
            else:
                result.append(self.text[self.pos])
                self.pos += 1
        
        if self.pos < len(self.text):
            self.pos += 1  # 跳过结束引号
        return ''.join(result)
    
    def _read_word(self) -> str:
        """读取单词（标识符或运算符）"""
        start = self.pos
        
        # 读取字母、数字、下划线、点号
        while self.pos < len(self.text) and (
            self.text[self.pos].isalnum() or 
            self.text[self.pos] in '._'
        ):
            self.pos += 1
        
        return self.text[start:self.pos]


class FalcoConditionParser:
    """Falco 条件解析器 - 递归下降解析"""
    
    def __init__(self, text: str):
        self.lexer = Lexer(text)
        self.tokens = self.lexer.tokenize()
        self.pos = 0
    
    def parse(self) -> ASTNode:
        """解析条件表达式"""
        if not self.tokens:
            raise ValueError("空条件表达式")
        
        result = self._parse_or_expression()
        
        if self.pos < len(self.tokens):
            raise ValueError(f"位置 {self.pos} 存在意外的 token: {self.tokens[self.pos]}")
        
        return result
    
    def _current_token(self) -> Optional[Token]:
        """获取当前 token"""
        if self.pos < len(self.tokens):
            return self.tokens[self.pos]
        return None
    
    def _consume(self, expected_type: Optional[str] = None) -> Token:
        """消费当前 token"""
        token = self._current_token()
        if token is None:
            raise ValueError("意外的表达式结束")
        
        if expected_type and token.type != expected_type:
            raise ValueError(f"期望 {expected_type}，得到 {token.type}")
        
        self.pos += 1
        return token
    
    def _parse_or_expression(self) -> ASTNode:
        """解析 OR 表达式（最低优先级）"""
        left = self._parse_and_expression()
        
        while self._current_token() and \
              self._current_token().type == 'LOGICAL' and \
              self._current_token().value == 'or':
            self._consume()  # 消费 'or'
            right = self._parse_and_expression()
            left = OrNode([left, right])
        
        return left
    
    def _parse_and_expression(self) -> ASTNode:
        """解析 AND 表达式"""
        left = self._parse_not_expression()
        
        while self._current_token() and \
              self._current_token().type == 'LOGICAL' and \
              self._current_token().value == 'and':
            self._consume()  # 消费 'and'
            right = self._parse_not_expression()
            left = AndNode([left, right])
        
        return left
    
    def _parse_not_expression(self) -> ASTNode:
        """解析 NOT 表达式"""
        if self._current_token() and \
           self._current_token().type == 'LOGICAL' and \
           self._current_token().value == 'not':
            self._consume()  # 消费 'not'
            child = self._parse_not_expression()
            return NotNode(child)
        
        return self._parse_comparison()
    
    def _parse_comparison(self) -> ASTNode:
        """解析比较表达式"""
        # 括号表达式
        if self._current_token() and \
           self._current_token().type == 'PAREN' and \
           self._current_token().value == '(':
            self._consume()  # 消费 '('
            expr = self._parse_or_expression()
            if self._current_token() and self._current_token().type == 'PAREN':
                self._consume()  # 消费 ')'
            return expr
        
        # 字段或宏
        field_token = self._consume('IDENTIFIER')
        field_or_macro = field_token.value
        
        # 检查是否为宏引用（没有后续操作符）
        if not self._current_token() or \
           (self._current_token().type == 'LOGICAL') or \
           (self._current_token().type == 'PAREN' and self._current_token().value == ')'):
            # 可能是宏引用
            return MacroNode(field_or_macro)
        
        # 运算符
        op_token = self._consume('OPERATOR')
        operator = op_token.value
        
        # 值
        value = self._parse_value()
        
        return ComparisonNode(field_or_macro, operator, value)
    
    def _parse_value(self):
        """解析值（字符串、数字或列表）"""
        token = self._current_token()
        
        if not token:
            raise ValueError("期望值")
        
        # 字符串
        if token.type == 'STRING':
            self._consume()
            return token.value
        
        # 列表 (val1, val2, ...)
        if token.type == 'PAREN' and token.value == '(':
            self._consume()  # 消费 '('
            values = []
            
            while True:
                val_token = self._consume()
                if val_token.type == 'STRING':
                    values.append(val_token.value)
                elif val_token.type == 'IDENTIFIER':
                    # 尝试转换为数字
                    try:
                        values.append(int(val_token.value))
                    except ValueError:
                        try:
                            values.append(float(val_token.value))
                        except ValueError:
                            values.append(val_token.value)
                
                # 检查是否结束
                next_token = self._current_token()
                if next_token and next_token.type == 'PAREN' and next_token.value == ')':
                    self._consume()  # 消费 ')'
                    break
                elif next_token and next_token.type == 'COMMA':
                    self._consume()  # 消费 ','
                else:
                    raise ValueError("列表中期望 ',' 或 ')'")
            
            return values
        
        # 标识符（可能是数字）
        if token.type == 'IDENTIFIER':
            self._consume()
            # 尝试转换为数字
            try:
                return int(token.value)
            except ValueError:
                try:
                    return float(token.value)
                except ValueError:
                    return token.value
        
        raise ValueError(f"意外的值 token: {token}")


# ============================================================================
# 列表和宏管理器
# ============================================================================

class ListManager:
    """列表管理器"""
    def __init__(self):
        self.lists: Dict[str, List[str]] = {}
    
    def add_list(self, name: str, items: List[str]):
        """添加列表"""
        self.lists[name] = items
        logger.debug(f"添加列表: {name} ({len(items)} 项)")
    
    def resolve(self, condition: str) -> str:
        """在条件中替换列表引用"""
        for name, items in self.lists.items():
            # 查找列表引用，例如: (list_name)
            pattern = rf'\({name}\)'
            if re.search(pattern, condition):
                # 替换为实际值，数字不加引号，字符串加引号
                formatted_items = []
                for item in items:
                    # 如果是数字，不加引号
                    if isinstance(item, (int, float)):
                        formatted_items.append(str(item))
                    # 如果是字符串但内容是数字，也不加引号
                    elif isinstance(item, str) and item.isdigit():
                        formatted_items.append(item)
                    # 否则加引号
                    else:
                        formatted_items.append(f'"{item}"')
                
                replacement = '(' + ', '.join(formatted_items) + ')'
                condition = re.sub(pattern, replacement, condition)
                logger.debug(f"替换列表 {name}: {pattern} -> {replacement}")
        
        return condition


class MacroManager:
    """宏管理器"""
    def __init__(self):
        self.macros: Dict[str, ASTNode] = {}
    
    def add_macro(self, name: str, ast: ASTNode):
        """添加宏"""
        self.macros[name] = ast
        logger.debug(f"添加宏: {name}")
    
    def expand(self, ast: ASTNode) -> ASTNode:
        """展开 AST 中的宏引用"""
        if isinstance(ast, MacroNode):
            # 展开宏
            if ast.name in self.macros:
                expanded = self.expand(self.macros[ast.name])
                ast.expanded = expanded
                return expanded
            else:
                # 不是宏，可能是字段引用，保持原样
                logger.debug(f"未找到宏: {ast.name}，保持为标识符")
                return ast
        
        elif isinstance(ast, AndNode):
            ast.children = [self.expand(child) for child in ast.children]
            return ast
        
        elif isinstance(ast, OrNode):
            ast.children = [self.expand(child) for child in ast.children]
            return ast
        
        elif isinstance(ast, NotNode):
            ast.child = self.expand(ast.child)
            return ast
        
        else:
            # 比较节点，不需要展开
            return ast


# ============================================================================
# 条件编译器（整合列表、宏、解析）
# ============================================================================

class ConditionCompiler:
    """条件编译器 - Falco 风格"""
    def __init__(self):
        self.list_manager = ListManager()
        self.macro_manager = MacroManager()
    
    def add_list(self, name: str, items: List[str]):
        """添加列表定义"""
        self.list_manager.add_list(name, items)
    
    def add_macro(self, name: str, condition: str):
        """添加宏定义"""
        # 解析宏的条件
        resolved_condition = self.list_manager.resolve(condition)
        parser = FalcoConditionParser(resolved_condition)
        ast = parser.parse()
        self.macro_manager.add_macro(name, ast)
    
    def compile(self, condition: str) -> ASTNode:
        """编译条件为 AST"""
        logger.debug(f"编译条件: {condition}")
        
        # 1. 替换列表
        resolved_condition = self.list_manager.resolve(condition)
        logger.debug(f"列表替换后: {resolved_condition}")
        
        # 2. 解析为 AST
        parser = FalcoConditionParser(resolved_condition)
        ast = parser.parse()
        logger.debug(f"解析 AST: {ast}")
        
        # 3. 展开宏
        ast = self.macro_manager.expand(ast)
        logger.debug(f"宏展开后: {ast}")
        
        return ast


# ============================================================================
# 事件标准化器
# ============================================================================

class EventNormalizer:
    """事件数据标准化器 - 将不同格式的事件数据转换为统一的 Falco 字段格式"""
    
    @staticmethod
    def normalize_event_data(event: Dict[str, Any]) -> Dict[str, Any]:
        """将 sysdig 事件数据标准化为 Falco 字段格式，适配 SysArmor 数据结构"""
        message = event.get('message', {})
        
        # 构建标准化的事件数据结构
        normalized = {
            # 事件基础信息
            'evt.type': message.get('evt.type', event.get('event_type', '')),
            'evt.time': message.get('evt.time', event.get('timestamp', '')),
            'evt.num': message.get('evt.num', 0),
            'evt.category': message.get('evt.category', event.get('event_category', '')),
            'evt.dir': message.get('evt.dir', '>'),
            'evt.args': message.get('evt.args', ''),
            
            # 进程信息
            'proc.name': message.get('proc.name', ''),
            'proc.exe': message.get('proc.exe', ''),
            'proc.exepath': message.get('proc.exe', ''),
            'proc.cmdline': message.get('proc.cmdline', ''),
            'proc.pname': message.get('proc.pname', ''),  # 父进程名称
            'proc.pcmdline': message.get('proc.pcmdline', ''),
            'proc.pid': message.get('proc.pid', 0),
            'proc.ppid': message.get('proc.ppid', 0),
            'proc.uid': message.get('proc.uid', 0),
            'proc.gid': message.get('proc.gid', 0),
            
            # 文件描述符信息
            'fd.name': message.get('fd.name', ''),
            'fd.nameraw': message.get('fd.name', ''),
            
            # 用户信息
            'user.name': EventNormalizer._get_user_name(message.get('proc.uid', 0)),
            'user.uid': message.get('proc.uid', 0),
            
            # 容器信息
            'container.id': message.get('container.id', 'host'),
            'container.name': message.get('container.name', ''),
            
            # 原始事件数据
            'message': message,
            'event': event
        }
        
        # 提取文件目录和文件名信息
        fd_name = normalized.get('fd.name', '')
        if fd_name:
            directory, filename = EventNormalizer._extract_file_info(fd_name)
            normalized['fd.directory'] = directory
            normalized['fd.filename'] = filename
        
        return normalized
    
    @staticmethod
    def _get_user_name(uid: int) -> str:
        """根据 UID 获取用户名"""
        system_users = {
            0: 'root',
            1: 'daemon',
            2: 'bin',
            65534: 'nobody'
        }
        return system_users.get(uid, f'user_{uid}')
    
    @staticmethod
    def _extract_file_info(fd_name: str) -> tuple:
        """从文件描述符名称中提取目录和文件名"""
        if not fd_name or '->' in fd_name:
            return '', ''
        
        import os
        directory = os.path.dirname(fd_name)
        filename = os.path.basename(fd_name)
        return directory, filename


# ============================================================================
# 威胁检测规则引擎（主类）
# ============================================================================

class ThreatDetectionRules:
    """威胁检测规则引擎
    基于 Falco 规则设计，支持条件字符串解析
    """
    
    def __init__(self, 
                 rules_file: str = "/opt/flink/configs/rules/threat_detection_rules.yaml",
                 enable_index: bool = True,
                 enable_stats: bool = True,
                 enable_rate_limit: bool = True,
                 max_alerts_per_minute: int = 100):
        """初始化规则引擎
        
        Args:
            rules_file: 规则文件路径
            enable_index: 是否启用事件类型索引（默认启用）
            enable_stats: 是否启用统计（默认启用）
            enable_rate_limit: 是否启用频率限制（默认启用）
            max_alerts_per_minute: 每规则每分钟最大告警数
        """
        self.rules = {}
        self.compiled_conditions = {}  # 存储编译后的 AST
        self.rule_groups = {}
        self.global_settings = {}
        self.compiler = ConditionCompiler()
        self.event_normalizer = EventNormalizer()
        
        # 优化特性开关
        self.enable_index = enable_index
        self.enable_stats = enable_stats
        self.enable_rate_limit = enable_rate_limit
        
        # 事件类型索引
        self.rules_by_event_type: Dict[str, Set[str]] = defaultdict(set)
        self.wildcard_rules: Set[str] = set()
        self.rule_priorities: Dict[str, int] = {}
        
        # 统计和限流
        self.stats_manager = StatsManager() if enable_stats else None
        self.rate_limiter = RateLimiter(max_alerts_per_minute) if enable_rate_limit else None
        
        # 加载规则
        self.load_rules(rules_file)
        
        # 构建索引
        if self.enable_index:
            self._build_event_type_index()
        
    def load_rules(self, rules_file: str):
        """加载威胁检测规则"""
        try:
            if os.path.exists(rules_file):
                with open(rules_file, 'r', encoding='utf-8') as f:
                    config = yaml.safe_load(f)
                
                # 1. 加载列表
                for list_def in config.get('lists', []):
                    self.compiler.add_list(list_def['list'], list_def['items'])
                
                # 2. 加载宏
                for macro_def in config.get('macros', []):
                    self.compiler.add_macro(macro_def['macro'], macro_def['condition'])
                
                # 3. 加载规则（支持 condition 字符串格式）
                for rule in config.get('rules', []):
                    if rule.get('enabled', True):
                        rule_id = rule['id']
                        self.rules[rule_id] = rule
                        
                        # 编译 condition 字符串（如果存在）
                        if 'condition' in rule and isinstance(rule['condition'], str):
                            try:
                                ast = self.compiler.compile(rule['condition'])
                                self.compiled_conditions[rule_id] = ast
                                logger.debug(f"规则 {rule_id} 编译成功: {ast}")
                            except Exception as e:
                                logger.error(f"规则 {rule_id} 编译失败: {e}")
                        # 兼容旧的字典格式
                        elif 'condition' in rule and isinstance(rule['condition'], dict):
                            # 保持原有的字典格式支持
                            pass
                
                # 加载规则组
                self.rule_groups = config.get('rule_groups', {})
                
                # 加载全局设置
                self.global_settings = config.get('global_settings', {})
                
                logger.info(f"✅ 加载了 {len(self.rules)} 个威胁检测规则")
                logger.info(f"📋 列表: {len(self.compiler.list_manager.lists)} 个")
                logger.info(f"🔧 宏: {len(self.compiler.macro_manager.macros)} 个")
                logger.info(f"📋 规则组: {list(self.rule_groups.keys())}")
            else:
                logger.warning(f"规则文件不存在: {rules_file}，使用默认规则")
                self._load_default_rules()
                
        except Exception as e:
            logger.error(f"加载规则失败: {e}，使用默认规则")
            self._load_default_rules()
    
    def _load_default_rules(self):
        """加载默认规则 - Falco 样式条件字符串"""
        # 添加默认列表
        self.compiler.add_list('sensitive_files', [
            '/etc/passwd', '/etc/shadow', '/etc/sudoers'
        ])
        self.compiler.add_list('shell_binaries', [
            'bash', 'sh', 'zsh', 'fish'
        ])
        
        # 添加默认宏
        self.compiler.add_macro('open_read', 'evt.type in (open, openat, openat2)')
        self.compiler.add_macro('open_write', 'evt.type in (open, openat) and fd.name contains "w"')
        
        # 定义规则
        self.rules = {
            "suspicious_tmp_execution": {
                "id": "suspicious_tmp_execution",
                "name": "可疑临时目录程序执行",
                "category": "suspicious_activity",
                "severity": "high",
                "base_score": 85,
                "condition": 'evt.type in (execve, execveat) and (proc.exe startswith "/tmp/" or proc.exe startswith "/dev/shm/")'
            },
            "sensitive_file_access": {
                "id": "sensitive_file_access",
                "name": "敏感文件访问检测",
                "category": "file_access",
                "severity": "medium",
                "base_score": 70,
                "condition": 'open_read and fd.name in (sensitive_files)'
            }
        }
        
        # 编译所有规则
        for rule_id, rule in self.rules.items():
            if 'condition' in rule and isinstance(rule['condition'], str):
                try:
                    ast = self.compiler.compile(rule['condition'])
                    self.compiled_conditions[rule_id] = ast
                except Exception as e:
                    logger.error(f"默认规则 {rule_id} 编译失败: {e}")
        
        logger.info("✅ 加载了默认威胁检测规则")
    
    def _build_event_type_index(self):
        """构建事件类型索引"""
        logger.info("🔨 开始构建事件类型索引...")
        
        for rule_id, rule in self.rules.items():
            # 提取规则优先级
            priority = self._get_rule_priority(rule)
            self.rule_priorities[rule_id] = priority
            
            # 分析规则条件，提取事件类型
            event_types = self._extract_event_types(rule_id)
            
            if not event_types:
                # 无法确定事件类型，加入通配规则
                self.wildcard_rules.add(rule_id)
                logger.debug(f"  规则 {rule_id} -> 通配规则")
            else:
                # 为每个事件类型添加规则索引
                for evt_type in event_types:
                    self.rules_by_event_type[evt_type].add(rule_id)
                logger.debug(f"  规则 {rule_id} -> 事件类型 {event_types}")
        
        logger.info("✅ 事件类型索引构建完成:")
        logger.info(f"  - 总规则数: {len(self.rules)}")
        logger.info(f"  - 事件类型数: {len(self.rules_by_event_type)}")
        logger.info(f"  - 通配规则数: {len(self.wildcard_rules)}")
        
        # 打印每个事件类型的规则数量
        for evt_type in sorted(self.rules_by_event_type.keys()):
            rule_count = len(self.rules_by_event_type[evt_type])
            logger.info(f"  - {evt_type}: {rule_count} 个规则")
    
    def _get_rule_priority(self, rule: Dict) -> int:
        """获取规则优先级（数字越小优先级越高）"""
        severity_to_priority = {
            'critical': 1,
            'high': 2,
            'medium': 3,
            'low': 4,
            'info': 5
        }
        severity = rule.get('severity', 'medium')
        return severity_to_priority.get(severity, 3)
    
    def _extract_event_types(self, rule_id: str) -> Set[str]:
        """从规则 AST 中提取事件类型"""
        event_types = set()
        
        if rule_id not in self.compiled_conditions:
            return event_types
        
        ast = self.compiled_conditions[rule_id]
        self._traverse_ast_for_event_types(ast, event_types)
        
        return event_types
    
    def _traverse_ast_for_event_types(self, node: ASTNode, event_types: Set[str]):
        """递归遍历 AST 提取事件类型"""
        if isinstance(node, ComparisonNode):
            # 检查是否为事件类型条件
            if node.field in ('evt.type', 'event_type', 'evt.category'):
                if node.operator in ('=', '=='):
                    event_types.add(str(node.value))
                elif node.operator == 'in':
                    if isinstance(node.value, (list, tuple)):
                        event_types.update(str(v) for v in node.value)
        
        elif isinstance(node, AndNode):
            for child in node.children:
                self._traverse_ast_for_event_types(child, event_types)
        
        elif isinstance(node, OrNode):
            for child in node.children:
                self._traverse_ast_for_event_types(child, event_types)
        
        elif isinstance(node, NotNode):
            # NOT: 无法优化，不提取事件类型
            pass
        
        elif isinstance(node, MacroNode):
            if node.expanded:
                self._traverse_ast_for_event_types(node.expanded, event_types)
    
    def evaluate_event(
        self, 
        event: Dict[str, Any],
        strategy: RuleMatchStrategy = RuleMatchStrategy.ALL
    ) -> List[Dict[str, Any]]:
        """评估事件是否触发威胁检测规则
        
        Args:
            event: 原始事件数据
            strategy: 规则匹配策略
                - ALL: 匹配所有规则（默认）
                - FIRST: 匹配第一个规则即停止
                - HIGHEST: 只返回最高优先级的规则
        
        Returns:
            告警列表
        """
        start_time = time.time()
        
        # 标准化事件数据结构
        normalized_event = self.event_normalizer.normalize_event_data(event)
        
        # 使用缓存包装
        cached_event = CachedEventData(normalized_event)
        
        # 获取需要检查的规则
        if self.enable_index:
            rules_to_check = self._get_indexed_rules(normalized_event)
        else:
            rules_to_check = list(self.rules.keys())
        
        # 按优先级排序规则
        sorted_rules = sorted(
            rules_to_check,
            key=lambda rid: self.rule_priorities.get(rid, 999)
        )
        
        logger.debug(f"🎯 检查 {len(sorted_rules)}/{len(self.rules)} 个规则")
        
        # 根据策略执行匹配
        if strategy == RuleMatchStrategy.ALL:
            alerts = self._match_all_rules(sorted_rules, cached_event, event)
        elif strategy == RuleMatchStrategy.FIRST:
            alerts = self._match_first_rule(sorted_rules, cached_event, event)
        elif strategy == RuleMatchStrategy.HIGHEST:
            alerts = self._match_highest_priority(sorted_rules, cached_event, event)
        else:
            alerts = []
        
        # 记录统计
        if self.enable_stats:
            elapsed_ms = (time.time() - start_time) * 1000
            self.stats_manager.record_event(len(alerts))
            logger.debug(f"⏱️  事件评估耗时: {elapsed_ms:.2f}ms")
        
        return alerts
    
    def _get_indexed_rules(self, normalized_event: Dict) -> List[str]:
        """获取需要检查的规则 ID 列表"""
        # 获取事件类型
        event_type = normalized_event.get('evt.type') or \
                     normalized_event.get('event_type', 'unknown')
        
        # 收集需要检查的规则
        rules_to_check = set(self.wildcard_rules)  # 始终包含通配规则
        
        # 添加该事件类型的专属规则
        if event_type in self.rules_by_event_type:
            rules_to_check.update(self.rules_by_event_type[event_type])
        
        # 也检查事件类别
        event_category = self._get_event_category(event_type)
        if event_category and event_category in self.rules_by_event_type:
            rules_to_check.update(self.rules_by_event_type[event_category])
        
        return list(rules_to_check)
    
    def _get_event_category(self, event_type: str) -> Optional[str]:
        """根据事件类型推断事件类别"""
        file_events = {'open', 'openat', 'openat2', 'read', 'write', 'close', 'unlink'}
        process_events = {'execve', 'execveat', 'fork', 'clone', 'exit'}
        network_events = {'socket', 'connect', 'bind', 'listen', 'accept', 'send', 'recv'}
        
        if event_type in file_events:
            return 'file'
        elif event_type in process_events:
            return 'process'
        elif event_type in network_events:
            return 'network'
        
        return None
    
    def _match_all_rules(
        self, 
        rule_ids: List[str], 
        cached_event: CachedEventData, 
        original_event: Dict
    ) -> List[Dict[str, Any]]:
        """匹配所有规则"""
        alerts = []
        for rule_id in rule_ids:
            rule = self.rules[rule_id]
            matched, eval_time = self._match_rule_timed(rule_id, cached_event, rule)
            
            if self.enable_stats:
                self.stats_manager.record_evaluation(rule_id, matched, eval_time)
            
            if matched:
                # 检查频率限制
                if self.enable_rate_limit and self.rate_limiter.should_drop_alert(rule_id):
                    continue
                
                alert = self._create_alert(original_event, rule)
                alerts.append(alert)
        
        return alerts
    
    def _match_first_rule(
        self, 
        rule_ids: List[str], 
        cached_event: CachedEventData, 
        original_event: Dict
    ) -> List[Dict[str, Any]]:
        """匹配第一个规则即停止（短路评估）"""
        for rule_id in rule_ids:
            rule = self.rules[rule_id]
            matched, eval_time = self._match_rule_timed(rule_id, cached_event, rule)
            
            if self.enable_stats:
                self.stats_manager.record_evaluation(rule_id, matched, eval_time)
            
            if matched:
                # 检查频率限制
                if self.enable_rate_limit and self.rate_limiter.should_drop_alert(rule_id):
                    continue
                
                alert = self._create_alert(original_event, rule)
                return [alert]
        
        return []
    
    def _match_highest_priority(
        self, 
        rule_ids: List[str], 
        cached_event: CachedEventData, 
        original_event: Dict
    ) -> List[Dict[str, Any]]:
        """只返回最高优先级的规则"""
        alerts = []
        highest_priority = None
        
        for rule_id in rule_ids:
            rule = self.rules[rule_id]
            priority = self.rule_priorities.get(rule_id, 999)
            
            # 如果已经找到更高优先级的规则，跳过当前规则
            if highest_priority is not None and priority > highest_priority:
                continue
            
            matched, eval_time = self._match_rule_timed(rule_id, cached_event, rule)
            
            if self.enable_stats:
                self.stats_manager.record_evaluation(rule_id, matched, eval_time)
            
            if matched:
                # 检查频率限制
                if self.enable_rate_limit and self.rate_limiter.should_drop_alert(rule_id):
                    continue
                
                alert = self._create_alert(original_event, rule)
                
                # 如果是更高优先级，清空之前的告警
                if highest_priority is None or priority < highest_priority:
                    alerts = [alert]
                    highest_priority = priority
                # 如果是相同优先级，添加到列表
                elif priority == highest_priority:
                    alerts.append(alert)
        
        return alerts
    
    def _match_rule_timed(
        self, 
        rule_id: str, 
        event_data: Union[Dict, CachedEventData], 
        rule: Dict
    ) -> tuple:
        """匹配规则并计时
        
        Returns:
            (matched: bool, eval_time_ms: float)
        """
        start = time.time()
        matched = self._match_rule(rule_id, event_data, rule)
        eval_time_ms = (time.time() - start) * 1000
        return matched, eval_time_ms
    
    def _match_rule(self, rule_id: str, event_data: Dict, rule: Dict) -> bool:
        """检查事件是否匹配规则"""
        try:
            # 优先使用编译后的 AST（Falco 风格）
            if rule_id in self.compiled_conditions:
                ast = self.compiled_conditions[rule_id]
                return ast.evaluate(event_data)
            
            # 兼容旧的字典格式条件
            elif 'condition' in rule and isinstance(rule['condition'], dict):
                return self._evaluate_dict_condition(rule['condition'], event_data)
            
            # 兼容旧的关键词和正则格式
            event_str = json.dumps(event_data, ensure_ascii=False)
            
            keywords = rule.get('keywords', [])
            for keyword in keywords:
                if keyword in event_str:
                    return True
            
            patterns = rule.get('patterns', [])
            for pattern in patterns:
                if re.search(pattern, event_str, re.IGNORECASE):
                    return True
            
            return False
            
        except Exception as e:
            logger.debug(f"规则匹配异常 {rule_id}: {e}")
            return False
    
    def _evaluate_dict_condition(self, condition: Dict, event_data: Dict) -> bool:
        """评估字典格式的条件（向后兼容）"""
        try:
            if 'and' in condition:
                return all(self._evaluate_dict_condition(sub_cond, event_data) for sub_cond in condition['and'])
            elif 'or' in condition:
                return any(self._evaluate_dict_condition(sub_cond, event_data) for sub_cond in condition['or'])
            elif 'not' in condition:
                return not self._evaluate_dict_condition(condition['not'], event_data)
            elif 'field' in condition:
                field_path = condition['field']
                operator = condition['operator']
                expected_value = condition.get('value', condition.get('values'))
                
                # 创建临时比较节点评估
                comp_node = ComparisonNode(field_path, operator, expected_value)
                return comp_node.evaluate(event_data)
            
            return False
        except Exception as e:
            logger.debug(f"字典条件评估异常: {e}")
            return False
    
    def _create_alert(self, event: Dict, rule: Dict) -> Dict[str, Any]:
        """创建告警事件"""
        now = datetime.utcnow()
        
        # 计算风险评分
        base_score = rule.get('base_score', 50)
        score_multiplier = rule.get('score_multiplier', 1.0)
        final_score = min(100, int(base_score * score_multiplier))
        
        # 确定严重程度
        severity = rule.get('severity', 'medium')
        if final_score >= 90:
            severity = 'critical'
        elif final_score >= 70:
            severity = 'high'
        elif final_score >= 50:
            severity = 'medium'
        else:
            severity = 'low'
        
        alert = {
            "@timestamp": now.isoformat() + 'Z',
            "alert": {
                "id": str(uuid.uuid4()),
                "type": "rule_based_detection",
                "category": rule.get('category', 'unknown'),
                "severity": severity,
                "risk_score": final_score,
                "confidence": 0.8,
                "rule": {
                    "id": rule['id'],
                    "name": rule.get('name', ''),
                    "description": rule.get('description', ''),
                    "title": f"{rule.get('name', 'Unknown Threat')}: {event.get('event_type', 'unknown')}",
                    "mitigation": f"检查 {rule.get('category', 'unknown')} 相关活动",
                    "references": [f"SysArmor Rule: {rule['id']}"]
                },
                "evidence": {
                    "event_type": event.get('event_type', ''),
                    "process_name": event.get('message', {}).get('proc.name', ''),
                    "process_cmdline": event.get('message', {}).get('proc.cmdline', ''),
                    "file_path": event.get('message', {}).get('fd.name', ''),
                    "network_info": event.get('message', {}).get('net.sockaddr', {})
                }
            },
            "event": {
                "raw": {
                    "event_id": event.get('event_id', ''),
                    "timestamp": event.get('timestamp', ''),
                    "source": event.get('source', 'auditd'),
                    "message": event.get('message', {})
                }
            },
            "timing": {
                "created_at": now.isoformat() + 'Z',
                "processed_at": now.isoformat() + 'Z'
            },
            "metadata": {
                "collector_id": event.get('collector_id', ''),
                "host": event.get('host', 'unknown'),
                "source": "sysarmor-threat-detector",
                "processor": "flink-events-to-alerts"
            }
        }
        
        return alert
    
    def print_stats(self):
        """打印统计信息"""
        if self.enable_stats:
            self.stats_manager.print_stats()
        else:
            logger.warning("统计功能未启用")
    
    def get_stats(self) -> Dict[str, Any]:
        """获取统计信息"""
        if self.enable_stats:
            return self.stats_manager.get_stats()
        return {}
    
    def print_index_stats(self):
        """打印索引统计信息"""
        if not self.enable_index:
            logger.warning("索引功能未启用")
            return
        
        print("\n" + "="*80)
        print("事件类型索引统计")
        print("="*80)
        print(f"总规则数:                {len(self.rules)}")
        print(f"索引的事件类型数:        {len(self.rules_by_event_type)}")
        print(f"通配规则数:              {len(self.wildcard_rules)}")
        
        if self.rules_by_event_type:
            total_indexed = sum(len(rules) for rules in self.rules_by_event_type.values())
            avg_per_type = total_indexed / len(self.rules_by_event_type)
            print(f"平均每事件类型规则数:    {avg_per_type:.2f}")
        
        print("="*80)
        
        # 计算预估性能提升
        if len(self.wildcard_rules) > 0 and self.rules_by_event_type:
            avg_check = len(self.wildcard_rules) + avg_per_type
            speedup = len(self.rules) / avg_check if avg_check > 0 else 1
            print(f"预估性能提升:            {speedup:.1f}x")
        
        print("="*80 + "\n")

ThreatDetectionEngine = ThreatDetectionRules

# ============================================================================
# 测试代码
# ============================================================================


if __name__ == "__main__":
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )
    
    print("🚀 测试威胁检测规则引擎 - 使用真实规则和数据")
    print("=" * 100)
    
    # 获取脚本所在目录的绝对路径
    script_dir = os.path.dirname(os.path.abspath(__file__))
    
    # 构建规则文件和数据文件的路径
    rules_file = os.path.join(script_dir, "../configs/rules/threat_detection_rules.yaml")
    data_file = os.path.join(
        script_dir, 
        "../../../data/kafka-exports/sysarmor.events.audit_20251023_203740/sysarmor.events.audit_20251023_203740.jsonl"
    )
    
    print(f"📋 规则文件: {rules_file}")
    print(f"📄 数据文件: {data_file}")
    print("=" * 100)
    
    # 创建引擎（启用所有优化）
    print("\n⚙️  初始化威胁检测引擎...")
    engine = ThreatDetectionRules(
        rules_file=rules_file,
        enable_index=True,
        enable_stats=True,
        enable_rate_limit=True,
        max_alerts_per_minute=100
    )
    
    # 打印索引统计
    engine.print_index_stats()
    
    # 加载测试事件数据
    print("\n📥 加载测试事件数据...")
    test_events = []
    
    if os.path.exists(data_file):
        with open(data_file, 'r', encoding='utf-8') as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if line:
                    try:
                        event = json.loads(line)
                        test_events.append(event)
                    except json.JSONDecodeError as e:
                        logger.warning(f"跳过无效JSON (行 {line_num}): {e}")
        
        print(f"✅ 成功加载 {len(test_events)} 个事件")
    else:
        print(f"❌ 数据文件不存在: {data_file}")
        print("使用默认测试事件...")
        test_events = [
            {
                "event_id": "test-1",
                "timestamp": "2025-10-22T10:00:00Z",
                "event_type": "execve",
                "message": {
                    "evt.type": "execve",
                    "proc.name": "bash",
                    "proc.exe": "/tmp/malicious.sh",
                    "proc.cmdline": "/tmp/malicious.sh --payload",
                    "proc.pid": 12345
                }
            }
        ]
    
    # 测试不同匹配策略
    print("\n" + "=" * 100)
    print("🧪 测试不同匹配策略")
    print("=" * 100)
    
    strategies_results = {}
    
    for strategy in RuleMatchStrategy:
        print(f"\n{'='*40} 策略: {strategy.value.upper()} {'='*40}")
        
        total_alerts = 0
        alert_by_severity = {'critical': 0, 'high': 0, 'medium': 0, 'low': 0, 'info': 0}
        alert_by_category = {}
        matched_events = 0
        
        start_time = time.time()
        
        for idx, event in enumerate(test_events):
            alerts = engine.evaluate_event(event, strategy=strategy)
            
            if alerts:
                matched_events += 1
                total_alerts += len(alerts)
                
                # 统计告警
                for alert in alerts:
                    severity = alert['alert']['severity']
                    category = alert['alert']['category']
                    
                    if severity in alert_by_severity:
                        alert_by_severity[severity] += 1
                    
                    alert_by_category[category] = alert_by_category.get(category, 0) + 1
                
                # 显示前5个匹配事件的详情
                if matched_events <= 5:
                    print(f"\n  事件 #{idx+1}: {event.get('event_id')} ({event.get('event_type')})")
                    for alert in alerts:
                        rule = alert['alert']['rule']
                        severity = alert['alert']['severity']
                        score = alert['alert']['risk_score']
                        print(f"    🚨 [{severity.upper():8s}] {rule['name']} (评分: {score})")
        
        elapsed_time = time.time() - start_time
        
        # 保存结果
        strategies_results[strategy.value] = {
            'total_events': len(test_events),
            'matched_events': matched_events,
            'total_alerts': total_alerts,
            'alerts_by_severity': alert_by_severity,
            'alerts_by_category': alert_by_category,
            'elapsed_time': elapsed_time
        }
        
        # 打印策略统计
        print(f"\n  📊 策略统计:")
        print(f"     总事件数:        {len(test_events)}")
        print(f"     匹配事件数:      {matched_events} ({matched_events/len(test_events)*100:.1f}%)")
        print(f"     总告警数:        {total_alerts}")
        print(f"     处理耗时:        {elapsed_time:.2f}s")
        print(f"     平均耗时/事件:   {elapsed_time/len(test_events)*1000:.2f}ms")
        
        print(f"\n  按严重程度分布:")
        for severity in ['critical', 'high', 'medium', 'low', 'info']:
            count = alert_by_severity.get(severity, 0)
            if count > 0:
                print(f"     {severity.upper():8s}: {count}")
        
        if alert_by_category:
            print(f"\n  按类别分布:")
            for category, count in sorted(alert_by_category.items(), key=lambda x: x[1], reverse=True):
                print(f"     {category:25s}: {count}")
    
    # 策略对比
    print("\n" + "=" * 100)
    print("📊 策略性能对比")
    print("=" * 100)
    print(f"\n{'策略':<10s} | {'匹配事件':<10s} | {'总告警':<10s} | {'耗时(s)':<10s} | {'ms/事件':<10s}")
    print("-" * 100)
    
    for strategy_name, results in strategies_results.items():
        matched = results['matched_events']
        alerts = results['total_alerts']
        elapsed = results['elapsed_time']
        ms_per_event = elapsed / results['total_events'] * 1000
        
        print(f"{strategy_name:<10s} | {matched:<10d} | {alerts:<10d} | {elapsed:<10.2f} | {ms_per_event:<10.2f}")
    
    # 打印规则引擎统计
    print("\n" + "=" * 100)
    print("� 规则引擎性能统计")
    print("=" * 100)
    engine.print_stats()
    
    # 生成测试报告
    print("\n" + "=" * 100)
    print("📝 测试总结")
    print("=" * 100)
    print(f"✅ 测试完成！")
    print(f"   - 加载规则数:     {len(engine.rules)}")
    print(f"   - 测试事件数:     {len(test_events)}")
    print(f"   - 测试策略数:     {len(RuleMatchStrategy)}")
    
    # 推荐策略
    print(f"\n💡 策略推荐:")
    all_result = strategies_results.get('all', {})
    first_result = strategies_results.get('first', {})
    highest_result = strategies_results.get('highest', {})
    
    print(f"   - 完整分析(ALL):      {all_result.get('total_alerts', 0)} 个告警, "
          f"{all_result.get('elapsed_time', 0):.2f}s")
    print(f"   - 快速响应(FIRST):    {first_result.get('total_alerts', 0)} 个告警, "
          f"{first_result.get('elapsed_time', 0):.2f}s (推荐生产环境)")
    print(f"   - 降噪过滤(HIGHEST):  {highest_result.get('total_alerts', 0)} 个告警, "
          f"{highest_result.get('elapsed_time', 0):.2f}s")
    
    print("\n" + "=" * 100)

