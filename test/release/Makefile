IMAGES ?= ubuntu2204 ubuntu2404 debian12
RUN_ID ?= $(shell date -u +%Y%m%dT%H%M%SZ)

.DEFAULT_GOAL := help
.PHONY: doctor test test-contract help

doctor:
	IMAGES="$(IMAGES)" RUN_ID="$(RUN_ID)" bash doctor.sh

test-contract:
	bash test-contract.sh

test: doctor
	IMAGES="$(IMAGES)" RUN_ID="$(RUN_ID)" bash run.sh

help:
	@echo "SysArmor public release container tests:"
	@echo "  make doctor        检查 Docker、eBPF/BTF 和公开安装地址"
	@echo "  make test-contract 检查测试矩阵静态契约"
	@echo "  make test          串行测试 Ubuntu 22.04/24.04 与 Debian 12"
	@echo ""
	@echo "可选参数:"
	@echo "  SYSARMOR_INSTALL_URL=https://.../install.sh"
	@echo "  IMAGES='ubuntu2204 debian12' RUN_ID=my-run"
