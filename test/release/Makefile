IMAGES ?= ubuntu2204 ubuntu2404 debian12
RUN_ID ?= $(shell date -u +%Y%m%dT%H%M%SZ)
URL ?=
FRESH_DOWNLOAD ?= 1
RELEASE_PROXY_URL ?= https://gh-proxy.org

.DEFAULT_GOAL := help
.PHONY: doctor test help

doctor:
	URL="$(URL)" IMAGES="$(IMAGES)" RUN_ID="$(RUN_ID)" FRESH_DOWNLOAD="$(FRESH_DOWNLOAD)" RELEASE_PROXY_URL="$(RELEASE_PROXY_URL)" bash doctor.sh

test: doctor
	URL="$(URL)" IMAGES="$(IMAGES)" RUN_ID="$(RUN_ID)" FRESH_DOWNLOAD="$(FRESH_DOWNLOAD)" RELEASE_PROXY_URL="$(RELEASE_PROXY_URL)" bash run.sh

help:
	@echo "SysArmor public release container tests:"
	@echo "  make doctor        检查 Docker、eBPF/BTF 和公开安装地址"
	@echo "  make test          串行测试 Ubuntu 22.04/24.04 与 Debian 12"
	@echo ""
	@echo "可选参数:"
	@echo "  URL=https://.../install.sh"
	@echo "  IMAGES='ubuntu2204 debian12' RUN_ID=my-run FRESH_DOWNLOAD=0"
	@echo "  RELEASE_PROXY_URL=https://gh-proxy.org（直连失败时使用）"
