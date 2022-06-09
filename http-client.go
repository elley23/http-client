package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

//downloadFile,做个http client从固定的网站下载文件，支持断点续传，下载的文件保存在当前目录

var durl = "https://dl.google.com/go/go1.10.3.darwin-amd64.pkg"

func downloadFileWithRange() {
	srcUrl, err := url.ParseRequestURI(durl)
	check(err)

	filename := path.Base(srcUrl.Path)
	tmpFile := filename + "_bak"
	//fmt.Println("[*]Filename:" + filename)
	//fmt.Println("[*]tmpFile:" + tmpFile)
	var count, total int64 = 0, 0
	var countf, totalf float64 = 0.0001, 10000
	bfb := "0%"
	//var thisCount int64 = 0

	file1, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	check(err)
	defer file1.Close()
	fileBak, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_RDWR, os.ModePerm)
	check(err)
	/*n1, err := fileBak.Stat()
	check(err)
	if n1.Size() > 0 {
		bp = 1
	}*/

	//读取临时记录下载情况的文件
	fileBak.Seek(0, io.SeekStart)
	buf := make([]byte, 100, 100)
	n1, err := fileBak.Read(buf)
	if err == io.EOF {
	}
	countStr := string(buf[:n1])
	countArr := strings.Split(countStr, "/")
	if len(countArr) >= 3 {
		count, _ = strconv.ParseInt(countArr[0], 10, 64)
		total, _ = strconv.ParseInt(countArr[1], 10, 64)
		bfb = countArr[2]
	}

	ntStart := time.Now()
	nt := time.Now().Format("2006-01-02 03:04:05")
	fmt.Println(fmt.Sprintf("[%s] 开始下载，已下载：%d，总共：%d，进度：%s", nt, count, total, bfb))

	//resp, err := http.Get(durl)
	//check(err)
	//defer resp.Body.Close()
	myClient := http.Client{} //{Timeout: time.Second * 60}
	req, err := http.NewRequest(http.MethodGet, durl, nil)
	check(err)
	if count == 0 {
		req.Header.Add("Range", "bytes=0-")
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", count, total))
	}
	resp, err := myClient.Do(req)
	check(err)
	defer resp.Body.Close()

	//用ioutil读取文件，如果断网，文件读取失败,不能panic,否则不能实现断点续传了
	//panic: read tcp 192.168.0.103:55182->120.253.255.33:443: wsarecv: An established connection was aborted
	//by the software in your host machine.

	//resp header
	contentRange := resp.Header.Get("Content-Range")
	totalRange := strings.Split(contentRange, "/")
	if len(totalRange) >= 2 {
		total, _ = strconv.ParseInt(totalRange[1], 10, 64)
	} else {
		value := resp.Header.Get("Content-Length")
		total, _ = strconv.ParseInt(value, 10, 64)
		//fmt.Printf("the content-length: %d\n", total)
	}

	//print resp headers
	/*headers := resp.Header
	for header := range headers {
		v := headers[header]
		println(header + "=" + strings.Join(v, "|"))
	}*/

	//resp body
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("ioutil read resp.body failed, please check your network..")
		//panic("Network failure!..")
	}
	file1.Seek(count, io.SeekStart)
	n2, err := file1.Write(data)
	check(err)

	count += int64(n2)
	countf = float64(count)
	totalf = float64(total)
	bfb = fmt.Sprintf("%.2f%", countf/totalf*100)
	fmt.Println(fmt.Sprintf("本次读取了：%d, 总共已下载：%d, 文件总大小：%d, 下载进度：%s", n2, count, total, bfb))

	w := strconv.Itoa(int(count)) + "/" + strconv.Itoa(int(total)) + "/" + bfb
	fileBak.Seek(0, io.SeekStart)
	fileBak.WriteString(w)
	if count == total {
		ntEnd := time.Now()
		nt := time.Now().Format("2006-01-02 03:04:05")
		fmt.Printf("[%s] 文件下载完毕...耗时[%v]\n", nt, ntEnd.Sub(ntStart))
		fileBak.Close()
	}
	return

}
func printRespInfo(resp *http.Response) {
	fmt.Println("Response Status:=", resp.Status)
	headers := resp.Header
	for header := range headers {
		v := headers[header]
		println(header + "=" + strings.Join(v, "|"))
	}
}

//实现goroutine处理大文件下载
func downloadFileGo() {
	srcUrl, err := url.ParseRequestURI(durl)
	if err != nil {
		panic("ParseRequestURI failure!")
	}
	filename := path.Base(srcUrl.Path)

	//发生HTTP HEAD，看server是否支持Range
	myClient := http.Client{}
	req, err := http.NewRequest(http.MethodHead, durl, nil)
	if err != nil {
		panic("http.NewRequest HEAD failure..")
	}
	req.Header.Add("Range", "bytes=0-0")
	resp, err := myClient.Do(req)
	if err != nil {
		panic("myClient.Do: HTTP HEAD send failure..")
	}
	defer resp.Body.Close()

	//print resp headers
	//printRespInfo(resp)

	//resp header
	v := resp.Header.Get("Accept-Ranges")
	if v != "bytes" {
		downloadFileNoRange(filename, durl)
		return
	}

	//取得文件大小
	var total int64
	contentRange := resp.Header.Get("Content-Range")
	totalRange := strings.Split(contentRange, "/")
	if len(totalRange) >= 2 {
		total, _ = strconv.ParseInt(totalRange[1], 10, 64)
	}

	//并发处理下载文件
	downloadFileGoroutine(filename, total, durl)

}

//不支持断点下传的服务器
func downloadFileNoRange(filename string, url string) {
	ntStart := time.Now()
	file1, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		panic("Open file failure..")
	}
	defer file1.Close()

	resp, err := http.Get(url)
	if err != nil {
		panic("http.Get failure..")
	}
	defer resp.Body.Close()
	v := resp.Header.Get("Content-Length")
	contentLength, _ := strconv.ParseInt(v, 10, 64)

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic("ioutil.Readall(resp.Body) failure...")
	}
	n, err := file1.Write(data)
	if err != nil {
		panic("file.Write failure...")
	}
	if n != int(contentLength) {
		fmt.Println("The file size maybe wrong...")
	}

	ntEnd := time.Now()
	fmt.Printf("共用时：%v\n", ntEnd.Sub(ntStart))
}

func downloadOneFile(url string, filename string, rangeStart, rangeEnd int) {
	var rangeValue string

	defer wg.Done()

	if rangeStart <= rangeEnd {
		rangeValue = fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd)
	} else if rangeEnd == 0 {
		rangeValue = fmt.Sprintf("bytes=%d-", rangeStart)
	} else if rangeStart > rangeEnd {
		fmt.Println("Range error...")
		return
	}

	//创建、发生http.get
	myClient := http.Client{}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Println("http new request failure..")
		return
	}
	req.Header.Add("Range", rangeValue)
	resp, err := myClient.Do(req)
	if err != nil {
		fmt.Println("http get failure..")
		return
	}
	defer resp.Body.Close()

	// 创建硬盘文件，名字为 源文件 + 第 number 个协程
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		fmt.Println("Create file failure...")
		return
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		fmt.Println("copy resp.body to file failure...")
		return
	}

}

var goroutine_number int = 10
var wg = sync.WaitGroup{}

func downloadFileGoroutine(filename string, size int64, url string) {
	ntStart := time.Now()
	eachFileLen := int(size) / goroutine_number

	//临时文件目录
	//myDir, _ := os.Getwd()
	//tmpDir := fmt.Sprintf("%s/templates/", myDir)
	//os.Mkdir(tmpDir, 0777)
	//defer os.Remove(tmpDir)
	err := os.MkdirAll("./templates", 0777)
	if err != nil {
		fmt.Printf("%s", err)
	} else {
		//fmt.Print("创建目录成功!")
	}
	defer os.RemoveAll("./templates")

	//file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	//if err != nil {
	//	println(err)
	//}
	//println(file)

	//启动并发
	wg.Add(goroutine_number)
	rangeStart := 0
	for i := 0; i < goroutine_number; i++ {
		tmpFilename := fmt.Sprintf("templates/%s_%d", filename, i)
		rangeEnd := rangeStart + eachFileLen
		if i == goroutine_number-1 {
			rangeEnd = 0
		}

		go downloadOneFile(url, tmpFilename, rangeStart, rangeEnd)
		rangeStart += eachFileLen + 1
	}

	wg.Wait()

	var offset int64 = 0
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		fmt.Println("mergeFiles: create destination file failure...")
		return
	}
	defer file.Close()
	for i := 0; i < goroutine_number; i++ {
		tmpFilename := fmt.Sprintf("templates/%s_%d", filename, i)
		n, err := mergeFiles(file, offset, tmpFilename)
		if err != nil {
			fmt.Println("merge files failure...")
			return
		}
		offset += n
	}

	ntEnd := time.Now()
	fmt.Printf("共用时：%v\n", ntEnd.Sub(ntStart))

	/*err = os.RemoveAll("./templates")
	if err != nil {
		fmt.Println("remove all failure...")
		fmt.Println(err)
	}*/
}

func mergeFiles(file *os.File, offset int64, srcFilename string) (n int64, err error) {
	srcFile, err := os.Open(srcFilename)
	if err != nil {
		fmt.Println("mergeFiles: open templates file failure...")
		return 0, err
	}
	defer srcFile.Close()

	file.Seek(offset, io.SeekStart)
	n1, err := io.Copy(file, srcFile)
	if err != nil {
		fmt.Println("mergeFiles: copy templates file failure...")
		return 0, err

	}
	return n1, nil
}
