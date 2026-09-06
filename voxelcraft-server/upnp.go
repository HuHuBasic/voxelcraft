package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// tryUPnP 尝试通过路由器 UPnP 自动映射 TCP 端口到本机（最佳努力，失败不影响运行）。
func tryUPnP(port int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[UPnP] 异常: %v", r)
		}
	}()
	loc, err := discoverIGD()
	if err != nil {
		log.Printf("[UPnP] 未发现支持 UPnP 的路由器（可忽略，手动端口映射或用隧道即可）: %v", err)
		return
	}
	ctrl, err := getControlURL(loc)
	if err != nil {
		log.Printf("[UPnP] 获取控制地址失败: %v", err)
		return
	}
	if err := addPortMapping(ctrl, port); err != nil {
		log.Printf("[UPnP] 添加端口映射失败: %v", err)
		return
	}
	log.Printf("[UPnP] 已自动将路由器 TCP/%d 映射到本机（公网端口相同）", port)
}

func discoverIGD() (string, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	msg := []byte("M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n")

	addr, _ := net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	for i := 0; i < 2; i++ {
		_, _ = conn.WriteToUDP(msg, addr)
	}
	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		text := string(buf[:n])
		if strings.Contains(text, "LOCATION:") {
			for _, line := range strings.Split(text, "\r\n") {
				if strings.HasPrefix(strings.ToUpper(line), "LOCATION:") {
					return strings.TrimSpace(line[len("LOCATION:"):]), nil
				}
			}
		}
	}
	return "", fmt.Errorf("timeout")
}

func getControlURL(location string) (string, error) {
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(location)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	desc := buf.String()

	// 找 WANIPConnection / WANPPPConnection 服务
	idx := strings.Index(desc, "WANIPConnection")
	if idx < 0 {
		idx = strings.Index(desc, "WANPPPConnection")
	}
	if idx < 0 {
		return "", fmt.Errorf("未找到 WAN 连接服务")
	}
	// 取该 service 块
	start := strings.LastIndex(desc[:idx], "<service>")
	if start < 0 {
		start = 0
	}
	end := strings.Index(desc[start:], "</service>")
	if end < 0 {
		return "", fmt.Errorf("服务块解析失败")
	}
	block := desc[start : start+end+len("</service>")]

	ctrlURL := extractTag(block, "controlURL")
	if ctrlURL == "" {
		return "", fmt.Errorf("无 controlURL")
	}
	if strings.HasPrefix(ctrlURL, "http") {
		return ctrlURL, nil
	}
	// 相对路径，拼接 base
	base := location
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[:i]
	}
	return base + ctrlURL, nil
}

func extractTag(s, tag string) string {
	open := "<" + tag + ">"
	closeT := "</" + tag + ">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	j := strings.Index(s, closeT)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(s[i+len(open) : j])
}

func addPortMapping(controlURL string, port int) error {
	lan := getLocalIP()
	soap := fmt.Sprintf(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body>
<u:AddPortMapping xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
<NewRemoteHost></NewRemoteHost>
<NewExternalPort>%d</NewExternalPort>
<NewProtocol>TCP</NewProtocol>
<NewInternalPort>%d</NewInternalPort>
<NewInternalClient>%s</NewInternalClient>
<NewEnabled>1</NewEnabled>
<NewPortMappingDescription>VoxelCraft</NewPortMappingDescription>
<NewLeaseDuration>0</NewLeaseDuration>
</u:AddPortMapping>
</s:Body>
</s:Envelope>`, port, port, lan)

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("POST", controlURL, strings.NewReader(soap))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPACTION", "\"urn:schemas-upnp-org:service:WANIPConnection:1#AddPortMapping\"")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
