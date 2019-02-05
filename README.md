# autotmp
背景：网上购买的数控恒温冰箱很不靠谱，正负温差在2度左右
解法思路：
1. 米家蓝牙温湿度计1个
2. 米家智能插座(wifi版)
3. 带蓝牙模块的wifi设备（raspberry pi或者yeelight语音助手）
4. 当室温较高时，冰箱设置为较低制冷模式。当温度高于设定温度时，开启冰箱制冷。当温度低于目标温度时，关闭冰箱，利用自然温度回升。
5. 反之设置为制热模式

小米智能家庭解法：
1. 放置一个蓝牙温度计在冰箱内
2. 将冰箱的电源插在智能插座上
3. 将以上两个设备连接网络
4. 在小米智能家庭（米家App）设置智能场景，实现解法思路中4或者5
5. 由于小米智能家庭中蓝牙网关上报存在较大延迟，约10分钟（如果是温湿度传感器更高延迟）。正负温差约在1度左右

本文解法：
1. 将上述解法中的依托智能场景替换为本程序
2. 通过接收蓝牙广播，获取到目标蓝牙温度计的温度
3. 判断温度，然后根据情况发送智能插座控制命令，在局域网内完成
4. 本人root了yeelight语音助手，将本程序编译到了助手上运行
5. GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" && upx autotmp && scp autotmp root@192.168.50.199:~/
6. 在助手上配置restartd命令，保持autotmp始终运行：autotmp "/root/autotmp" "/root/autotmp >> /root/autotmp.log &" "/bin/echo 'autotmp is running'"
7. 配置crontab，每隔30分钟杀掉autotmp（hcidump老是在40分钟后卡死，没有深入研究，用这个方法hack下）:*/30 * * * * killall -9 autotmp
8. 几个运行参数配置：http://192.168.50.199:8080/autotmp?cmd=conf
auto_off_freq:10	#每隔10秒检查控温逻辑
counter_for_avg:10	#统计10次平均温度
mac_filter:5DA8DEA8654C #蓝牙温度计的mac地址：可以通过安卓上安装BLE scanner获取mac地址，但需要反转
type_filter:1004	#蓝牙温度计固定为1004
plug_ip:192.168.50.143	#智能插座ip地址
plug_token:{your token}	#智能插座token,可以用https://github.com/nickw444/miio-go去抓取
tmp_ctrl_mode:cool	#控温模式冷却（cool）或者加热（heat）
tmp_ctrl_time:10	#控温时间
tmp_will_reverse:2501	#控温阀值
9. 可以通过http://192.168.50.199:8080/autotmp?cmd=log获取日志
10.通过http://192.168.50.199:8080/autotmp?cmd=set&key=tmp_ctrl_time&val=100来控制控温时间，其他参数类似
11.以上ip根据实际情况调整
12.温差基本控制在0.2以内
13.如果有两个蓝牙温度计，可以自行修改，控制控温模式
14.等有时间，加上两个蓝牙温度计，结合深度学习，可以训练一套模型，实现真正智能模式
