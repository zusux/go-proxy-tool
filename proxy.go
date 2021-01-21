package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	Proxy _proxy
	localIp = ""
	ProxyIPs []string
	lock sync.RWMutex
	del chan string
	add chan string
	read chan string
	isAdd chan bool
)

type _proxy struct{
	Debug bool `yaml:"debug"`
	TestIpApi string `yaml:"testIpApi"`
	ProxyApis []string `yaml:"proxyApis"`
	IsTicker bool `yaml:"isTicker"`
	TickTime int `yaml:"tickTime"`
	MinProxyNum int `yaml:"minProxyNum"`
	DialTimeout int `yaml:"dialTimeout"`
	Deadline int `yaml:"deadline"`
	TestDialTimeout int `yaml:"testDialTimeout"`
	TestDeadline  int `yaml:"testDeadline"`
	ProxyFile  string `yaml:"proxyFile"`
	IsMinProxy  bool `yaml:"isMinProxy"`
	MinProxyCheckTime  int `yaml:"minProxyCheckTime"`
}

//sock5代理
func SocksProxy(uri string) (string,error) {
	urli := url.URL{}
	urlproxy, _ := urli.Parse("http://"+getOneProxy())
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(urlproxy),
		},
	}
	rqt, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		doDebug(err,"SocksProxy->http.NewRequest")
		return "",err
	}

	rqt.Header.Add("User-Agent", "Lingjiang")
	//处理返回结果
	response, err := client.Do(rqt)
	if err != nil{
		doDebug(err,"SocksProxy->client.Do")
		return "",err
	}
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		doDebug(err,"SocksProxy->ioutil.ReadAll")
		return "",err
	}
	return string(body),nil
}

func doDebug(err error,position string){
	if Proxy.Debug && err != nil{
		log.Printf("[%s] %v\n",position,err)
	}
}

// 代理请求方法
func RequestProxy(url string) (string,error){
	total := 0
	for {
		var err error
		proxy := getOneProxy()
		if len(proxy)>0{
			headers := make(map[string]string,0)
			fmt.Println("proxy",proxy)
			body,err := HttpProxy(proxy,url,headers)
			if err != nil{
				if strings.Contains(err.Error(),"connection failed"){
					doDebug(err,"RequestProxy->HttpProxy")
					del <- proxy
				}
				total++
			}else{
				return body,nil
			}
		}
		if total > 3{
			doDebug(err,"total > 3")
			return "",err
		}
		time.Sleep(time.Millisecond * 300)
	}
}

//http代理
func HttpProxy(proxy string,uri string,headers map[string]string)(string,error) {

	dialTimeout := Proxy.DialTimeout
	deadline := Proxy.Deadline
	if uri == Proxy.TestIpApi{
		dialTimeout = Proxy.TestDialTimeout
		deadline = Proxy.TestDeadline
	}

	urli := url.URL{}
	urlproxy, _ := urli.Parse(fmt.Sprintf("http://%s",proxy))
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(urlproxy),
			Dial: func(netw, addr string) (net.Conn, error) {
				c, err := net.DialTimeout(netw, addr, time.Millisecond * time.Duration(dialTimeout)) //设置建立连接超时
				if err != nil {
					return nil, err
				}
				c.SetDeadline(time.Now().Add(time.Millisecond * time.Duration(deadline))) //设置发送接收数据超时
				return c, nil
			},
		},
	}
	rqt, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		doDebug(err,"HttpProxy->http.NewRequest")
		return "",err
	}
	for k,v := range headers{
		rqt.Header.Add(k, v)
	}
	//处理返回结果
	response, err := client.Do(rqt)
	if err != nil{
		doDebug(err,"HttpProxy->client.Do")
		return "",err
	}
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		doDebug(err,"HttpProxy->ioutil.ReadAll")
		return "",err
	}
	return string(body),nil
}

//本机IP
func HttpLocal(url string) (string,error) {
	client := &http.Client{}
	rqt, err := http.NewRequest("GET", url, nil)
	if err != nil {
		doDebug(err,"HttpLocal->http.NewRequest")
		return "",err
	}
	rqt.Header.Add("User-Agent", "Lingjiang")
	//处理返回结果
	response, err := client.Do(rqt)
	if err != nil{
		doDebug(err,"HttpLocal->client.Do")
		return "",nil
	}
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		doDebug(err,"HttpLocal->ioutil.ReadAll")
		return "",err
	}
	return string(body),nil
}
func toolIp(body string) string{
	var d  map[string]interface{}
	err := json.Unmarshal([]byte(body),&d)
	if err != nil{
		doDebug(err,"toolIp->json.Unmarshal")
	}
	ip,ok := d["ip"]
	if ok{
		return ip.(string)
	}else{
		return ""
	}
}
func toolLocalIp(){
	body,err := HttpLocal(Proxy.TestIpApi)
	if err != nil{
		doDebug(err,"toolLocalIp->HttpLocal")
		return
	}
	localIp = toolIp(body)
}

func toolTestProxy(proxy string) bool{
	headers := make(map[string]string,0)
	headers["User-Agent"] = "Lingjiang"
	body,err := HttpProxy(proxy,Proxy.TestIpApi,headers)
	if err != nil{
		doDebug(err,"toolTestProxy->HttpProxy")
		return false
	}
	ip := toolIp(body)
	if len(ip) >0 && ip != localIp{
		return true
	}
	return false
}

func checkProxyValid(){
	for _,v := range ProxyIPs{
		go func(vv string) {
			flag := toolTestProxy(vv)
			if vv == "" || !flag{
				del <- vv
			}else{
				doDebug(errors.New("检测代理正常!"),vv)
			}
		}(v)
	}

}

func getProxy(){
	for{
		flag := <- isAdd
		if flag{
			doDebug(errors.New("执行获取代理!"),"")
			getProxyIp()
		}
	}
}

func delProxy()  {
	for{
		proxy := <- del
		for k,v := range ProxyIPs{
			if v == proxy {
				ProxyIPs = append(ProxyIPs[:k], ProxyIPs[k+1:]...)
				doDebug(errors.New("删除代理IP!"),proxy)
				if len(ProxyIPs) == 0{
					isAdd <- true
				}
				break
			}
		}
		writeProxy()
	}
}
func addProxy()  {
	for{
		proxy := <- add
		if proxy != "" {
			doDebug(errors.New("添加代理IP!"),proxy)
			ProxyIPs = append(ProxyIPs, proxy)
			writeProxy()
		}
	}
}

func writeProxy(){
	doDebug(errors.New("写入文件"),Proxy.ProxyFile)
	file,err := os.Create(Proxy.ProxyFile)
	if err != nil{
		doDebug(err,"writeProxy->os.Create")
		return
	}
	defer file.Close()
	text := strings.Join(ProxyIPs,"\n")
	_,err = file.WriteString(text)
	if err != nil{
		doDebug(err,"file.WriteString")
	}
}
func readProxy(){
	doDebug(errors.New("读取文件"),Proxy.ProxyFile)
	file,err := os.Open(Proxy.ProxyFile)
	if err != nil{
		doDebug(err,"readProxy->os.Open")
		return
	}

	b := bufio.NewReader(file)
	for {
		line,err := b.ReadString('\n')
		lineStr := strings.Trim(string(line),"\r\n")
		lock.Lock()
		ProxyIPs = append(ProxyIPs,lineStr)
		lock.Unlock()
		if err != nil || io.EOF == err{
			break
		}
	}
}


func getProxyIp(){

	for _,proxyApi := range Proxy.ProxyApis{
		fmt.Println(fmt.Sprintf("代理地址: [%s]",proxyApi))
		body,err := HttpLocal(proxyApi)
		if err != nil{
			doDebug(err,"getProxyIp->HttpLocal")
			continue
		}
		if strings.Contains(body,"code"){
			fmt.Println(body)
			if strings.Contains(body,"白名单"){
				doDebug(errors.New("设置白名单"),"getProxyIp")
				setWhite()
			}
			continue
		}

		arr := strings.Split(body,"\r\n")
		for _,v := range arr{
			if len(v) != 0{
				add <- v
			}
		}
		break
	}
}

func distributeProxy(){
	for{
		for _,v := range ProxyIPs{
			doDebug(errors.New("分配代理:"),v)
			read <- v
		}
	}
}
func getOneProxy() string {
   return <-read
}


func loadConfig(){
	content, err := ioutil.ReadFile("./proxy.yaml")
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(content,&Proxy)
	if err != nil{
		log.Fatal(err)
	}
	//fmt.Printf("%#v",Proxy)
}

func init()  {
	fmt.Println("执行proxy init")
	loadConfig()


	ProxyIPs = make([]string,0)
	add = make(chan string ,1)
	del = make(chan string,1)
	read = make(chan string,1)
	isAdd = make(chan bool,1)

	//获取本机ip
	toolLocalIp()
	//读取代理文件 proxy.txt
	readProxy()
	//启动添加代理
	go addProxy()
	//启动删除代理
	go delProxy()
	//启动获取代理
	go getProxy()
	//启动分配代理
	go distributeProxy()

	//定期检查最小代理数
	if Proxy.IsMinProxy{
		go func() {
			for{
				if len(ProxyIPs) < Proxy.MinProxyNum{
					isAdd <- true
				}
				doDebug(errors.New("目前最小代理数"),fmt.Sprintf("%d",len(ProxyIPs)))
				time.Sleep(time.Millisecond * time.Duration(Proxy.MinProxyCheckTime))
			}
		}()
	}


	//定期检查代理有效性
	if Proxy.IsTicker{
		Ticker := time.NewTicker(time.Millisecond * time.Duration(Proxy.TickTime))
		go func() {
			for{
				select{
				case <- Ticker.C:
					go checkProxyValid()
				}
			}
		}()
	}
}

func setWhite()  {
	if len(localIp) >0{
		uri := "http://wapi.http.linkudp.com/index/index/save_white?neek=142065&appkey=707db7f83961a95d2c24676313a2e7ab&white="+localIp
		s,err := HttpLocal(uri)
		doDebug(err,s)
	}

}

func main() {


	select{

	}
}

