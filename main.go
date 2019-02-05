package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"time"
	"strings"

	"github.com/vkorn/go-miio"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"net/http"
	"net/url"
)

const (
	mac_start = 14
	mac_end = 26
	v_type_start = 66
	v_type_end = 70
	v_start = 70
	v_end =74
)

func hcidump(data chan []byte, quit chan bool) {
	wg := new(sync.WaitGroup)
	cmd := exec.Command("hcidump", "-R")
	c := make(chan struct{})
	wg.Add(1)
	go func(cmd *exec.Cmd, c chan struct{}) {
		defer wg.Done()
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
		<-c
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
				case <-quit:
					fmt.Println("recv: quit")
				return
				case data<-scanner.Bytes():
			}
		}
	}(cmd, c)
	c<-struct{}{}
	fmt.Println("hcidump start")
	cmd.Start()
	fmt.Println("hcidump started")
	wg.Wait()
	fmt.Println("hcidump exited")
}

func parseRawPkt(raw string) (m,t string, v int64) {
	if len(raw) > v_end {
		mac := raw[mac_start:mac_end]
		tmp_t := raw[v_type_start:v_type_end]
		s, err := strconv.ParseInt(raw[v_start+2:v_end]+raw[v_start:v_start+2], 16, 32)
		if err != nil {
			fmt.Println(raw, err)
			return
		}
		m,t,v = mac,tmp_t,s
	}
	return
}

var (
	total int64
	count int64
)
func autoTmp(m,t string, v int64) {
	if m == mac_filter && t == type_filter {
		count = count + 1
		total = total + v
		if count == counter_for_avg {
			avg := float64(total) / float64(count)
			total = 0
			count = 0
			mux.Lock()
			//制冷模式下，高出指定温度开始启动电源制冷；制热模式下，低于指定温度开始启动电源制热；
			if (tmp_ctrl_mode == "cool" && avg > tmp_ctrl_limit) || (tmp_ctrl_mode == "heat" && avg < tmp_ctrl_limit) {
				if onTime == zeroTime {
					log(time.Now().Format("20060102 15:04:05"), " ", tmp_ctrl_mode+"_on", " ", plg.On(), " ", avg)
					onTime = time.Now()
				}
			} else {
				log(time.Now().Format("20060102 15:04:05"), " ", tmp_ctrl_mode+"_off", " ", plg.Off(), " ", avg)
				onTime = zeroTime
			}
			mux.Unlock()
		}
	}
}

func ctrl() {
	data := make(chan []byte, 1024)
	quit := make(chan bool, 1)
	go hcidump(data, quit)
	t := time.NewTicker(time.Minute * 15)
	lines:=""
	r:=strings.NewReplacer(" ","","\n","")
	for {
		select {
		case d := <-data:
			if d[0]=='>' {
				raw:=r.Replace(lines)
				m, t, v := parseRawPkt(raw)
				autoTmp(m, t, v)
				lines=string(d[1:])
			} else {
				lines=lines+string(d)
			}
		case <-t.C:
			quit <- true
			go hcidump(data, quit)
		}
	}
}

var (
	zeroTime time.Time = time.Unix(0, 0)
	onTime time.Time = zeroTime
	mux *sync.Mutex = new(sync.Mutex)
)

func autoOff() {
	t:=time.NewTicker(time.Second*time.Duration(getWithDefaultInt("auto_off_freq", 10)))
	for {
		select {
		case c:=<-t.C:
			loadConf()
			mux.Lock()
			if onTime!=zeroTime {
				last := c.Sub(onTime)
				if last.Seconds() > float64(tmp_ctrl_time) {
					log(time.Now().Format("20060102 15:04:05")," ", tmp_ctrl_mode+"_off"," ", plg.Off()," ", "timer_off:"," ", tmp_ctrl_time)
					onTime= zeroTime
				}
			}
			mux.Unlock()
		}
	}
}

func log(val ...interface{}) {
	out:=fmt.Sprint(val...)
	fmt.Println(out)
	gdb.Put([]byte("log_"+time.Now().Format("20060102150405")), []byte(out), nil)
}

func getWithDefaultInt(key string, val int) int {
	vb,err:=gdb.Get([]byte("conf_"+key), nil)
	if err!=nil {
		vb=[]byte(strconv.Itoa(val))
		gdb.Put([]byte("conf_"+key), vb, nil)
	}
	v,e:=strconv.Atoi(string(vb))
	if e!=nil {
		return val
	}
	return v
}

func setKeyVal(key, val string) string {
	err:=gdb.Put([]byte("conf_"+key), []byte(val),nil)
	if err!=nil {
		return err.Error()
	}
	return val
}

func getWithDefault(key, dft string) string {
	vb,err:=gdb.Get([]byte("conf_"+key), nil)
	if err!=nil {
		vb=[]byte(dft)
		gdb.Put([]byte("conf_"+key), vb, nil)
	}
	return string(vb)
}

func iterLog(prefix string) []string {
	ret := make([]string, 0)
	iter := gdb.NewIterator(util.BytesPrefix([]byte("log_"+prefix)), nil)
	for iter.Next() {
		ret = append(ret, string(iter.Value()))
	}
	iter.Release()
	return ret
}

func iterConf() []string {
	ret := make([]string, 0)
	iter := gdb.NewIterator(util.BytesPrefix([]byte("conf_")), nil)
	for iter.Next() {
		ret = append(ret, string(iter.Key())[5:]+":"+string(iter.Value()))
	}
	iter.Release()
	return ret
}

var (
	tmp_ctrl_limit float64
	tmp_ctrl_time int64
	tmp_ctrl_mode string

	counter_for_avg int64

	plug_ip string
	plug_token string

	mac_filter string
	type_filter string
)
func loadConf() {
	tmp_ctrl_limit = float64(getWithDefaultInt("tmp_will_reverse",2501))/10
	counter_for_avg = int64(getWithDefaultInt("counter_for_avg",10))
	tmp_ctrl_time = int64(getWithDefaultInt("tmp_ctrl_time",10))
	tmp_ctrl_mode = getWithDefault("tmp_ctrl_mode", "cool")	//heat

	plug_ip = getWithDefault("plug_ip", "192.168.50.143")
	plug_token = getWithDefault("plug_token","????")
	mac_filter = getWithDefault("mac_filter", "5DA8DEA8654C")
	type_filter = getWithDefault("type_filter", "1004")
}

func httpServer() {
	helloHandler := func(w http.ResponseWriter, r *http.Request) {
		queryForm, err := url.ParseQuery(r.URL.RawQuery)
		fmt.Println(r.URL.RawQuery)
		if err == nil && len(queryForm["cmd"]) > 0 {
			cmd:=queryForm["cmd"][0]
			if cmd=="conf" {
				for _,v:=range iterConf() {
					fmt.Fprintf(w, "%s\n", v)
				}
			} else if cmd=="set" {
				keys:=queryForm["key"]
				vals:=queryForm["val"]
				if len(keys)>0 && len(vals)>0 {
					fmt.Fprintln(w, setKeyVal(keys[0], vals[0]))
				}
			} else {
				pres:=queryForm["pre"]
				pre:=time.Now().Format("2006010215")
				if len(pres)>0 {
					pre = pres[0]
				}
				for _, v := range iterLog(pre) {
					fmt.Fprintf(w, "%s\n", v)
				}
			}
		}
	}

	http.HandleFunc("/autotmp", helloHandler)
	fmt.Println(http.ListenAndServe(":8080", nil))
}

var plg *miio.Plug
var gdb *leveldb.DB
func main() {
	db, err := leveldb.OpenFile("/root/dbs", nil)
	if err !=nil {
		fmt.Println(err)
		return
	}
	defer db.Close()
	gdb = db
	loadConf()

	plug, err := miio.NewPlug(plug_ip, plug_token)
	if err != nil {
		fmt.Println(err)
		return
	}
	plg=plug
	go autoOff()
	go ctrl()
	httpServer()
}
