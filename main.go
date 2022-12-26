package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/text/encoding/simplifiedchinese"
)

var (
	ipReg *regexp.Regexp

	logPath string
	command string
	windows bool

	masterIp    string
	hostsIpPort []string
	hostsIp     []string
	hostsPort   []int
)

// 本程序的初始化函数
func init() {
	ipReg, _ = regexp.Compile(`((2(5[0-5]|[0-4]\d))|[0-1]?\d{1,2})(\.((2(5[0-5]|[0-4]\d))|[0-1]?\d{1,2})){3}`)
}

// 命令行提示的初始化
func flagInit() {
	flag.BoolVar(&windows, "w", true, "Whether to connect windows hosts.")
	flag.StringVar(&command, "c", "echo dstb-shell", "Your command to be executed")
	flag.StringVar(&logPath, "L", "/", "[/tmp/] determine whether to log, Path e.g ./, Forbidden /")
	flag.Usage = flagUsage
}

// 使用说明
func flagUsage() {
	fmt.Println("Dstb-shell is a program to run shell command over distributed operation system.")
	fmt.Println("Its full name is distributed-shell.")
	fmt.Println("Step1: edit the configure file below this folder and")
	fmt.Println("     : run this program to generate certificates.")
	fmt.Println("Step2: use the data/hosts_cert**cert_thing** to deploy the socat.")
	fmt.Println("     : you need to refer to the readme or the introductions.")
	fmt.Println("Step3: use ./dstb-shell -c `cmd line` [-] to control hosts.")
	flag.PrintDefaults()
}

func logSettup() {
	// set the formatflag of log
	// log.SetFlags(log.Lshortfile | log.LstdFlags)
	log.SetFlags(log.LstdFlags)
	// define the log writer
	if logPath != "/" {
		file := logPath + time.Now().Format("2006-01-02 15-04") + ".log"
		logFile, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0766)
		if err != nil {
			log.Fatal(err)
		}
		writers := []io.Writer{
			logFile,
			os.Stdout,
		}
		fileAndStdoutWriter := io.MultiWriter(writers...)
		log.SetOutput(fileAndStdoutWriter)
	} else {
		writers := []io.Writer{
			os.Stdout,
		}
		fileAndStdoutWriter := io.MultiWriter(writers...)
		log.SetOutput(fileAndStdoutWriter)
	}
}

func viperInit() {
	viper.SetConfigFile("./config/config.yaml") // 指定配置文件路径
	viper.SetConfigName("config")               // 配置文件名称(无扩展名)
	viper.SetConfigType("yaml")                 // 如果配置文件的名称中没有扩展名，则需要配置此项
	// viper.AddConfigPath("/etc/appname/")        // 查找配置文件所在的路径
	// viper.AddConfigPath("$HOME/.appname")       // 多次调用以添加多个搜索路径
	viper.AddConfigPath("./config/") // 还可以在工作目录中查找配置
	err := viper.ReadInConfig()      // 查找并读取配置文件
	if err != nil {                  // 处理读取配置文件的错误
		panic(fmt.Errorf("fatal error config file: %s", err))
	}
	masterIp = viper.GetString("master")
	hostsIpPort = viper.GetStringSlice("hosts")
	hostsIp = make([]string, len(hostsIpPort))
	hostsPort = make([]int, len(hostsIpPort))
	for i := 0; i < len(hostsIpPort); i++ {
		hostsIp[i] = strings.Split(hostsIpPort[i], ":")[0]
		hostsPort[i], _ = strconv.Atoi(strings.Split(hostsIpPort[i], ":")[1])
		if !ipReg.MatchString(hostsIp[i]) {
			panic(fmt.Errorf("fata error config IP"))
		}
	}
}

// 处理windows字符集,windows:GBK->UTF8,linux:UTF8->UTF8
func convertByte2String(bytes []byte) string {
	if windows {
		decodeBytes, _ := simplifiedchinese.GBK.NewDecoder().Bytes(bytes)
		return string(decodeBytes)
	} else {
		return string(bytes)
	}
}

//具有超时停止的的阻塞,命令，毫秒
func execShellTimeout(s string, timeout int) (string, error) {
	//函数返回一个*Cmd，用于使用给出的参数执行name指定的程序
	cmd := exec.Command("/bin/bash", "-c", s)

	//读取io.Writer类型的cmd.Stdout，再通过bytes.Buffer(缓冲byte类型的缓冲器)将byte类型转化为string类型(out.String():这是bytes类型提供的接口)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start starts the specified command but does not wait for it to complete.
	cmd.Start()
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()
	// 设置定时器
	after := time.After(time.Duration(timeout) * time.Millisecond)
	// select 本身是阻塞的
	select {
	case <-after:
		cmd.Process.Signal(syscall.SIGINT)
		// time.Sleep(10 * time.Millisecond)
		cmd.Process.Kill()
	case <-done:
		return convertByte2String(stdout.Bytes()), nil
	}
	return convertByte2String(stdout.Bytes()), fmt.Errorf("time out: %s", stderr.String())
}

//阻塞式的,执行外部shell命令的函数,等待执行完毕并返回标准输出
func execShell(s string) (string, error) {
	//函数返回一个*Cmd，用于使用给出的参数执行name指定的程序
	cmd := exec.Command("/bin/bash", "-c", s)

	//读取io.Writer类型的cmd.Stdout，再通过bytes.Buffer(缓冲byte类型的缓冲器)将byte类型转化为string类型(out.String():这是bytes类型提供的接口)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	//Run执行c包含的命令，并阻塞直到完成。  这里stdout被取出，cmd.Wait()无法正确获取stdin,stdout,stderr，则阻塞在那了
	err := cmd.Run()
	if err != nil {
		fmt.Println(err)
		fmt.Println(stderr.String())
		return stderr.String(), err
	}
	return stdout.String(), err
}

//阻塞式的,需要对shell标准输出的逐行实时进行处理的
func execCommand(commandName string, params []string) bool {
	//函数返回一个*Cmd，用于使用给出的参数执行name指定的程序
	cmd := exec.Command(commandName, params...)

	//显示运行的命令
	fmt.Println(cmd.Args)
	//StdoutPipe方法返回一个在命令Start后与命令标准输出关联的管道。Wait方法获知命令结束后会关闭这个管道，一般不需要显式的关闭该管道。
	stdout, err := cmd.StdoutPipe()

	if err != nil {
		fmt.Println(err)
		return false
	}

	cmd.Start()
	//创建一个流来读取管道内内容，这里逻辑是通过一行一行的读取的
	reader := bufio.NewReader(stdout)

	//实时循环读取输出流中的一行内容
	for {
		line, err2 := reader.ReadString('\n')
		if err2 != nil || io.EOF == err2 {
			break
		}
		fmt.Println(line)
	}

	//阻塞直到该命令执行完成，该命令必须是被Start方法开始执行的
	cmd.Wait()
	return true
}

//查找文件是否存在
func isFileExist(s string) bool {
	_, err := os.Stat(s)
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

//生成自签名证书
func genCert(ip string, location string) {
	crt := isFileExist(fmt.Sprintf("%s%s.crt", location, ip))
	key := isFileExist(fmt.Sprintf("%s%s.key", location, ip))
	pem := isFileExist(fmt.Sprintf("%s%s.pem", location, ip))
	if crt && key && pem {
		return
	}
	cmd := fmt.Sprintf(`openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes -keyout %s%s.key -out %s%s.crt -subj "/CN=%s" -addext "subjectAltName=DNS:%s"`, location, ip, location, ip, ip, ip)
	execShell(cmd)
	cmd = fmt.Sprintf(`cat %s%s.* >> %s%s.pem`, location, ip, location, ip)
	execShell(cmd)
}

// 根据配置文件生成全部证书
func genCertAll() {
	genCert(masterIp, "./data/master_cert/")
	for i := 0; i < len(hostsIp); i++ {
		genCert(hostsIp[i], "./data/hosts_cert/")
	}
}

// 通过利用socat实现远程shell命令执行，使用最安全的ssl加密和证书认证
func execSocat(c string, serverIp string, serverPort int, clientPem string, serverCrt string, wg *sync.WaitGroup) {
	defer wg.Done()
	cmd := fmt.Sprintf(`echo "%s" | socat - OPENSSL:%s:%d,cert=%s,cafile=%s`, c, serverIp, serverPort, clientPem, serverCrt)
	s, err := execShellTimeout(cmd, 3000)
	// log.Println("Goroutine...")
	if err != nil {
		log.Printf("[%s][error]\n", serverIp)
		log.Println(s)
		log.Println(err)
	} else {
		log.Printf("[%s][correct]\n", serverIp)
		log.Println(s)
	}
}

func main() {
	flagInit()
	flag.Parse()
	logSettup()
	viperInit()
	genCertAll()
	// 声明一个等待组
	var wg sync.WaitGroup
	// 任务循环
	for i := 0; i < len(hostsIpPort); i++ {
		wg.Add(1)
		go execSocat(command, hostsIp[i], hostsPort[i], "./data/master_cert/"+masterIp+".pem", "./data/hosts_cert/"+hostsIp[i]+".crt", &wg)
	}
	// 等待所有的任务完成
	wg.Wait()
}
