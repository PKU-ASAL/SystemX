# VM Topology Deploy Cache

结论：这个目录保存 `vm-topology` 的部署输入缓存，不保存测试结果。

`start-vm.sh` 会生成：

- `platform/`：轻量平台部署包，包含 deployments、mTLS runtime 证书和本机构建出的 manager/gateway/worker 二进制。
- `images/`：VM 需要的 Docker 镜像包和 `images.manifest`。manifest 未变化时会复用本地包；manager VM 已有同版本包时会跳过 2G 级上传和 `docker load`。

`platform/` 和 `images/` 都是可再生的大文件目录，已被 `.gitignore` 忽略。

测试运行输出仍统一写入：

```text
test/.results/
```
