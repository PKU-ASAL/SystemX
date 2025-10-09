FROM flink:1.18.1

# 完全禁用GPG验证和签名检查
RUN echo 'APT::Get::AllowUnauthenticated "true";' > /etc/apt/apt.conf.d/99allow-unauthenticated && \
    echo 'Acquire::Check-Valid-Until "false";' > /etc/apt/apt.conf.d/99disable-check-valid-until && \
    echo 'APT::Get::Assume-Yes "true";' > /etc/apt/apt.conf.d/99assume-yes && \
    echo 'Acquire::AllowInsecureRepositories "true";' > /etc/apt/apt.conf.d/99allow-insecure && \
    echo 'Acquire::AllowDowngradeToInsecureRepositories "true";' > /etc/apt/apt.conf.d/99allow-downgrade && \
    rm -rf /etc/apt/trusted.gpg.d/* && \
    rm -rf /var/lib/apt/lists/*

# Install Python (强制忽略所有验证)
RUN apt-get update -o Acquire::Check-Valid-Until=false -o APT::Get::AllowUnauthenticated=true -o Acquire::AllowInsecureRepositories=true --allow-insecure-repositories || true && \
    apt-get install -y --allow-unauthenticated --allow-downgrades --allow-remove-essential --allow-change-held-packages python3 python3-pip python3-dev || apt-get install -y --force-yes python3 python3-pip python3-dev && \
    rm -rf /var/lib/apt/lists/*

# Ensure python command points to python3
RUN ln -s /usr/bin/python3 /usr/bin/python

# 设置 pip 镜像源配置
RUN pip config set global.index-url https://pypi.org/simple && \
    pip config set global.extra-index-url "https://pypi.tuna.tsinghua.edu.cn/simple https://mirrors.aliyun.com/pypi/simple" && \
    pip config set install.trusted-host "pypi.org pypi.tuna.tsinghua.edu.cn mirrors.aliyun.com"

# install Java & updates (强制忽略所有验证)
RUN apt-get update -o Acquire::Check-Valid-Until=false -o APT::Get::AllowUnauthenticated=true -o Acquire::AllowInsecureRepositories=true --allow-insecure-repositories || true && \
    apt-get install -y --allow-unauthenticated --allow-downgrades --allow-remove-essential --allow-change-held-packages openjdk-21-jdk ant || apt-get install -y --force-yes openjdk-21-jdk ant && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

ENV JAVA_HOME=/usr/lib/jvm/java-21-openjdk-arm64

WORKDIR /opt/flink
COPY services/processor/requirements.txt .
RUN pip install --timeout 300 --retries 3 -i https://pypi.tuna.tsinghua.edu.cn/simple -r requirements.txt

# 复制并执行下载脚本
COPY services/processor/scripts/download_libs.sh .
RUN chmod +x ./download_libs.sh && ./download_libs.sh

# 创建 usr_jobs 目录并设置 Python Path
RUN mkdir -p /opt/flink/usr_jobs
ENV PYTHONPATH=/opt/flink/usr_jobs
