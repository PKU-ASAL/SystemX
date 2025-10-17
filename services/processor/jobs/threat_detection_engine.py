#!/usr/bin/env python3
"""
SysArmor Threat Detection Engine
威胁检测规则引擎
基于 Falco/Sysdig 规则引擎设计
支持 Falco 风格的条件字符串解析、列表和宏
"""

import os
import json
import logging
import re
import yaml
import uuid
from datetime import datetime
from typing import Dict, List, Optional, Any, Union
from abc import ABC, abstractmethod

logger = logging.getLogger(__name__)


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
    
    def evaluate(self, event_data: Dict) -> bool:
        # 从事件中提取字段值
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
    """威胁检测规则引擎 - 基于 Falco 规则设计，支持条件字符串解析"""
    
    def __init__(self, rules_file: str = "/opt/flink/configs/rules/threat_detection_rules.yaml"):
        self.rules = {}
        self.compiled_conditions = {}  # 存储编译后的 AST
        self.rule_groups = {}
        self.global_settings = {}
        self.compiler = ConditionCompiler()
        self.event_normalizer = EventNormalizer()
        self.load_rules(rules_file)
        
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
    
    def evaluate_event(self, event: Dict[str, Any]) -> List[Dict[str, Any]]:
        """评估事件是否触发威胁检测规则"""
        alerts = []
        
        # 标准化事件数据结构
        normalized_event = self.event_normalizer.normalize_event_data(event)
        
        for rule_id, rule in self.rules.items():
            if self._match_rule(rule_id, normalized_event, rule):
                alert = self._create_alert(event, rule)
                alerts.append(alert)
        
        return alerts
    
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
