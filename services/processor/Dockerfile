FROM flink:1.18.1

# 替换为清华 TUNA Ubuntu apt 镜像源
RUN sed -i 's@http://archive.ubuntu.com/ubuntu/@https://mirrors.tuna.tsinghua.edu.cn/ubuntu/@g' /etc/apt/sources.list && \
    sed -i 's@http://security.ubuntu.com/ubuntu/@https://mirrors.tuna.tsinghua.edu.cn/ubuntu/@g' /etc/apt/sources.list

# Install Python
RUN apt-get update -y && \
    apt-get install -y python3 python3-pip python3-dev && \
    rm -rf /var/lib/apt/lists/*

# Ensure python command points to python3
RUN ln -s /usr/bin/python3 /usr/bin/python

# 设置 pip 镜像源配置，添加备用源解决403问题
RUN pip config set global.index-url https://pypi.org/simple && \
    pip config set global.extra-index-url "https://pypi.tuna.tsinghua.edu.cn/simple https://mirrors.aliyun.com/pypi/simple" && \
    pip config set install.trusted-host "pypi.org pypi.tuna.tsinghua.edu.cn mirrors.aliyun.com"

# install Java & updates
RUN apt-get update && \
    apt-get install -y openjdk-21-jdk ant && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

ENV JAVA_HOME=/usr/lib/jvm/java-21-openjdk-arm64

WORKDIR /opt/flink
COPY services/processor/requirements.txt .
RUN pip install -i https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple -r requirements.txt

# 复制并执行下载脚本
COPY services/processor/scripts/download_libs.sh .
RUN chmod +x ./download_libs.sh && ./download_libs.sh

# 创建 usr_jobs 目录并设置 Python Path
RUN mkdir -p /opt/flink/usr_jobs
ENV PYTHONPATH=/opt/flink/usr_jobs
