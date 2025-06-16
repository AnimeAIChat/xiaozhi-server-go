# Centos 8下小智Server Go版本安装过程


# 1、下载安装go 1.24版本
```Bash
cd /tmp
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
```

然后设置环境变量：
```Bash
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc
```
上述只是临时生效，永久生效需修改 /etc/profile 文件

```Shell
vim /etc/profile
```

在最后面添加

```Bash
export GO111MODULE=on
export GOROOT=/usr/local/go
export GOPATH=/home/gopath
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin
```

使环境变量生效

```Shell
source /etc/profile
```

验证版本：
```Shell
go version
```
切换go源为国内代理
```Shell
go env -w GOPROXY=https://goproxy.cn,direct
```


# 2、下载安装最新opus

### ✅ 1. 安装 opus 和 opus-devel 包
```
sudo dnf install opus opus-devel  # CentOS 8+
```
如果你是 CentOS 7，用 yum：
```
sudo yum install opus opus-devel
```
opus-devel 包含了 .pc 文件（给 pkg-config 使用），头文件、链接库等。

### ✅ 2. 确保 pkg-config 能找到 opus.pc

一般安装好之后会自动放在 /usr/lib64/pkgconfig/opus.pc。

你可以检查：
```
pkg-config --cflags opus
```
如果没有报错就说明找到了。如果还是找不到，尝试设置环境变量：
```
export PKG_CONFIG_PATH=/usr/lib64/pkgconfig
```
但是这样安装好的opus不是最新的，不能用，

## ✅ 原因分析

- OPUS_GET_IN_DTX_REQUEST 是 libopus 中比较新的宏，旧版本的 libopus 不包含它。

- qrtc/opus-go 的绑定代码引用了这个宏，所以如果系统安装的是 较旧版本的 libopus，就会报错。

---

## ✅ 解决方法

### 方法一：升级 libopus（推荐）
```
pkg-config --modversion opus
```
如果低于 1.3.1（如 1.1 或 1.2），说明版本太旧。

# 安装依赖
```
sudo dnf install gcc make autoconf automake libtool
```
# 下载并构建
```
cd /tmp
git clone https://github.com/xiph/opus.git
cd opus
./autogen.sh
./configure
make -j$(nproc)
sudo make install
export PKG_CONFIG_PATH=/usr/local/lib/pkgconfig
```
可将它加入 .bashrc：
```
echo 'export PKG_CONFIG_PATH=/usr/local/lib/pkgconfig' >> ~/.bashrc
```
```
source ~/.bashrc
```
然后重试：
```
go clean -modcache
go run ./src/main.go
```
# 3、获取源码
```
git clone https://github.com/AnimeAIChat/xiaozhi-server-go
```
下载完成后，复制一份config.yaml到.config.yaml，并在配置中填好相应的key等信息，避免密钥泄漏

# 4、运行

运行

```
 go mod tidy
 go run ./src/main.go
```



