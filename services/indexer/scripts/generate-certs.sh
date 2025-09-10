#!/bin/bash
set -e

# SysArmor Indexer SSL 证书生成脚本
CERT_DIR="./configs/opensearch/certs"
OPENSEARCH_DN="/C=US/ST=CA/L=San Francisco/O=SysArmor/OU=Security/CN=opensearch"
ADMIN_DN="/C=US/ST=CA/L=San Francisco/O=SysArmor/OU=Security/CN=admin"

echo "🔐 生成 SysArmor Indexer SSL 证书..."

# 创建证书目录
mkdir -p $CERT_DIR

# 生成 CA 私钥
echo "📋 生成 CA 私钥..."
openssl genrsa -out $CERT_DIR/root-ca-key.pem 2048

# 生成 CA 证书
echo "📋 生成 CA 证书..."
openssl req -new -x509 -sha256 -key $CERT_DIR/root-ca-key.pem -out $CERT_DIR/root-ca.pem -days 3650 -subj "/C=US/ST=CA/L=San Francisco/O=SysArmor/OU=Security/CN=root-ca"

# 生成 Admin 私钥
echo "📋 生成 Admin 私钥..."
openssl genrsa -out $CERT_DIR/admin-key-temp.pem 2048

# 转换 Admin 私钥格式
echo "📋 转换 Admin 私钥格式..."
openssl pkcs8 -inform PEM -outform PEM -in $CERT_DIR/admin-key-temp.pem -topk8 -nocrypt -v1 PBE-SHA1-3DES -out $CERT_DIR/admin-key.pem

# 生成 Admin 证书签名请求
echo "📋 生成 Admin 证书签名请求..."
openssl req -new -key $CERT_DIR/admin-key.pem -out $CERT_DIR/admin.csr -subj "$ADMIN_DN"

# 生成 Admin 证书
echo "📋 生成 Admin 证书..."
openssl x509 -req -in $CERT_DIR/admin.csr -CA $CERT_DIR/root-ca.pem -CAkey $CERT_DIR/root-ca-key.pem -CAcreateserial -sha256 -out $CERT_DIR/admin.pem -days 3650

# 生成 Node 私钥
echo "📋 生成 Node 私钥..."
openssl genrsa -out $CERT_DIR/node-key-temp.pem 2048

# 转换 Node 私钥格式
echo "📋 转换 Node 私钥格式..."
openssl pkcs8 -inform PEM -outform PEM -in $CERT_DIR/node-key-temp.pem -topk8 -nocrypt -v1 PBE-SHA1-3DES -out $CERT_DIR/node-key.pem

# 生成 Node 证书签名请求
echo "📋 生成 Node 证书签名请求..."
openssl req -new -key $CERT_DIR/node-key.pem -out $CERT_DIR/node.csr -subj "$OPENSEARCH_DN"

# 创建 SAN 配置文件
echo "📋 创建 SAN 配置文件..."
cat > $CERT_DIR/node.conf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
C = US
ST = CA
L = San Francisco
O = SysArmor
OU = Security
CN = opensearch

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = opensearch
DNS.2 = localhost
DNS.3 = sysarmor-indexer-opensearch
DNS.4 = sysarmor-opensearch
IP.1 = 127.0.0.1
IP.2 = 0.0.0.0
EOF

# 生成 Node 证书
echo "📋 生成 Node 证书..."
openssl x509 -req -in $CERT_DIR/node.csr -CA $CERT_DIR/root-ca.pem -CAkey $CERT_DIR/root-ca-key.pem -CAcreateserial -sha256 -out $CERT_DIR/node.pem -extensions v3_req -extfile $CERT_DIR/node.conf -days 3650

# 清理临时文件
echo "🧹 清理临时文件..."
rm -f $CERT_DIR/admin-key-temp.pem $CERT_DIR/node-key-temp.pem $CERT_DIR/admin.csr $CERT_DIR/node.csr $CERT_DIR/node.conf

# 设置权限
echo "🔒 设置证书权限..."
chmod 600 $CERT_DIR/*.pem
chmod 644 $CERT_DIR/root-ca.pem $CERT_DIR/admin.pem $CERT_DIR/node.pem

echo "✅ SSL 证书生成完成!"
echo ""
echo "📋 证书文件列表:"
echo "  - CA 证书: $CERT_DIR/root-ca.pem"
echo "  - CA 私钥: $CERT_DIR/root-ca-key.pem"
echo "  - Admin 证书: $CERT_DIR/admin.pem"
echo "  - Admin 私钥: $CERT_DIR/admin-key.pem"
echo "  - Node 证书: $CERT_DIR/node.pem"
echo "  - Node 私钥: $CERT_DIR/node-key.pem"
echo ""
echo "🔑 证书信息:"
echo "  - Admin DN: $ADMIN_DN"
echo "  - Node DN: $OPENSEARCH_DN"
echo ""
echo "🚀 现在可以启动 SysArmor Indexer 服务了!"
echo "   运行: make up"
