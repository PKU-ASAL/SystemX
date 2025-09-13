#!/bin/bash

# SysArmor Dev-Zheng 分支迁移功能测试脚本
# 测试统一 Resources API 的各项功能

# 注意: 不使用 set -e，以便继续执行所有测试

# 颜色定义
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# 配置
readonly MANAGER_URL="${MANAGER_HOST:-localhost}:${MANAGER_PORT:-8080}"
readonly MANAGER_URL="http://${MANAGER_URL}"
readonly TEST_OUTPUT_DIR="./tests/migrations/outputs"
readonly TEST_COLLECTOR_DATA='{
    "hostname": "test-server-dev-zheng",
    "ip_address": "192.168.1.200",
    "os_type": "linux",
    "os_version": "Ubuntu 22.04",
    "deployment_type": "agentless"
}'

# 辅助函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# 检查服务健康状态
check_service_health() {
    log_info "检查 SysArmor Manager 服务状态..."
    
    if curl -s "${MANAGER_URL}/health" > /dev/null; then
        log_success "Manager 服务运行正常"
    else
        log_error "Manager 服务不可访问，请先启动服务: make up"
        exit 1
    fi
}

# 注册测试 Collector
register_test_collector() {
    log_info "注册测试 Collector..."
    
    local response=$(curl -s -X POST "${MANAGER_URL}/api/v1/collectors/register" \
        -H "Content-Type: application/json" \
        -d "${TEST_COLLECTOR_DATA}")
    
    if echo "$response" | grep -q '"success":true'; then
        COLLECTOR_ID=$(echo "$response" | grep -o '"collector_id":"[^"]*"' | cut -d'"' -f4)
        log_success "Collector 注册成功: ${COLLECTOR_ID}"
        return 0
    else
        log_error "Collector 注册失败: $response"
        return 1
    fi
}

# 测试脚本下载功能
test_script_download() {
    local deployment_type="$1"
    local script_name="$2"
    local expected_prefix="$3"
    
    log_info "测试 ${deployment_type} 脚本下载: ${script_name}"
    
    local url="${MANAGER_URL}/api/v1/resources/scripts/${deployment_type}/${script_name}?collector_id=${COLLECTOR_ID}"
    local output_file="${TEST_OUTPUT_DIR}/${expected_prefix}-${COLLECTOR_ID:0:8}.sh"
    
    local http_code=$(curl -s -w "%{http_code}" -o "$output_file" "$url")
    
    if [ "$http_code" = "200" ]; then
        local file_size=$(stat -c%s "$output_file" 2>/dev/null || echo "0")
        log_success "脚本下载成功: ${output_file} (${file_size} bytes)"
        
        # 验证文件内容
        if grep -q "$COLLECTOR_ID" "$output_file"; then
            log_success "脚本包含正确的 Collector ID"
        else
            log_warning "脚本中未找到 Collector ID"
        fi
        
        return 0
    else
        log_error "脚本下载失败: HTTP ${http_code}"
        cat "$output_file" 2>/dev/null || true
        return 1
    fi
}

# 测试配置下载功能
test_config_download() {
    local deployment_type="$1"
    local config_name="$2"
    local expected_prefix="$3"
    
    log_info "测试 ${deployment_type} 配置下载: ${config_name}"
    
    local url="${MANAGER_URL}/api/v1/resources/configs/${deployment_type}/${config_name}?collector_id=${COLLECTOR_ID}"
    local output_file="${TEST_OUTPUT_DIR}/${expected_prefix}-${COLLECTOR_ID:0:8}"
    
    local http_code=$(curl -s -w "%{http_code}" -o "$output_file" "$url")
    
    if [ "$http_code" = "200" ]; then
        local file_size=$(stat -c%s "$output_file" 2>/dev/null || echo "0")
        log_success "配置下载成功: ${output_file} (${file_size} bytes)"
        
        # 验证文件内容
        if grep -q "$COLLECTOR_ID" "$output_file"; then
            log_success "配置包含正确的 Collector ID"
        else
            log_warning "配置中未找到 Collector ID"
        fi
        
        return 0
    else
        log_error "配置下载失败: HTTP ${http_code}"
        cat "$output_file" 2>/dev/null || true
        return 1
    fi
}

# 测试二进制文件下载功能
test_binary_download() {
    log_info "测试二进制文件下载功能..."
    
    # 创建测试二进制文件
    local test_binary="${TEST_OUTPUT_DIR}/test-binary"
    echo "This is a test binary file" > "$test_binary"
    
    # 注意: 实际环境中需要将文件放到 data/dist/ 目录
    log_warning "二进制文件下载需要先将文件放置到 data/dist/ 目录"
    log_info "测试 URL: ${MANAGER_URL}/api/v1/resources/binaries/test-binary"
}

# 运行所有测试
run_all_tests() {
    local failed_tests=0
    local total_tests=0
    
    log_info "开始 Dev-Zheng 分支迁移功能测试..."
    echo "=================================="
    
    # 创建输出目录
    mkdir -p "$TEST_OUTPUT_DIR"
    
    # 检查服务健康状态
    check_service_health || exit 1
    
    # 注册测试 Collector
    register_test_collector || exit 1
    
    echo ""
    log_info "开始测试 Resources API 功能..."
    echo "=================================="
    
    # 测试 Agentless 脚本下载
    ((total_tests++))
    if test_script_download "agentless" "setup-terminal.sh" "setup-terminal"; then
        log_success "✅ Agentless 安装脚本测试通过"
    else
        log_error "❌ Agentless 安装脚本测试失败"
        ((failed_tests++))
    fi
    
    # 测试 Agentless 卸载脚本下载
    ((total_tests++))
    if test_script_download "agentless" "uninstall-terminal.sh" "uninstall-terminal"; then
        log_success "✅ Agentless 卸载脚本测试通过"
    else
        log_error "❌ Agentless 卸载脚本测试失败"
        ((failed_tests++))
    fi
    
    # 测试 Agentless 配置下载
    ((total_tests++))
    if test_config_download "agentless" "audit-rules" "audit-rules"; then
        log_success "✅ Agentless 配置下载测试通过"
    else
        log_error "❌ Agentless 配置下载测试失败"
        ((failed_tests++))
    fi
    
    # 测试 OpenTelemetry Collector 脚本下载
    ((total_tests++))
    if test_script_download "collector" "install.sh" "install-otelcol"; then
        log_success "✅ OpenTelemetry Collector 脚本测试通过"
    else
        log_error "❌ OpenTelemetry Collector 脚本测试失败"
        ((failed_tests++))
    fi
    
    # 测试 OpenTelemetry Collector 配置下载
    ((total_tests++))
    if test_config_download "collector" "cfg.yaml" "otelcol"; then
        log_success "✅ OpenTelemetry Collector 配置测试通过"
    else
        log_error "❌ OpenTelemetry Collector 配置测试失败"
        ((failed_tests++))
    fi
    
    # 测试二进制文件下载
    test_binary_download
    
    echo ""
    echo "=================================="
    log_info "测试结果汇总"
    echo "=================================="
    
    if [ $failed_tests -eq 0 ]; then
        log_success "🎉 所有测试通过! (${total_tests}/${total_tests})"
        log_info "测试文件保存在: ${TEST_OUTPUT_DIR}/"
        
        echo ""
        log_info "生成的文件列表:"
        ls -la "$TEST_OUTPUT_DIR/" | grep -v "^total" | while read line; do
            echo "  $line"
        done
        
        return 0
    else
        log_error "❌ ${failed_tests}/${total_tests} 个测试失败"
        return 1
    fi
}

# 清理测试文件
cleanup() {
    if [ -d "$TEST_OUTPUT_DIR" ]; then
        log_info "清理测试文件..."
        rm -rf "$TEST_OUTPUT_DIR"
        log_success "测试文件已清理"
    fi
}

# 显示帮助信息
show_help() {
    echo "SysArmor Dev-Zheng 分支迁移功能测试脚本"
    echo ""
    echo "用法:"
    echo "  $0 [选项]"
    echo ""
    echo "选项:"
    echo "  test     运行所有测试 (默认)"
    echo "  cleanup  清理测试文件"
    echo "  help     显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0 test      # 运行所有测试"
    echo "  $0 cleanup   # 清理测试文件"
}

# 主函数
main() {
    case "${1:-test}" in
        "test")
            run_all_tests
            ;;
        "cleanup")
            cleanup
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        *)
            log_error "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
}

# 脚本入口
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
