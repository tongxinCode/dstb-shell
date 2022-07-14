# DSTB-shell

distributed shell

分布式shell，使用socat实现分布式命令行批量执行

## 编译

```bsh
./configure
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/dstb-shell main.go
```

## 运行

### 控制端

```bsh
./build/dstb-shell -c `cmd line` [-]
```
### 受控端

其中server.pem是指受控端的证书文件，client.crt是指控制端的证书文件，证书文件可由本程序生成

```bsh
socat OPENSSL-LISTEN:44443,cert=server.pem,cafile=client.crt,fork EXEC:/bin/bash
```

```powershell
.\socat.exe OPENSSL-LISTEN:44443,cert=server.pem,cafile=client.crt,fork EXEC:'cmd.exe',pipes
```
