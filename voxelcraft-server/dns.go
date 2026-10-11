package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// cloudflareUpsertDNS 在 Cloudflare 上创建或更新一条 DNS 记录，实现"绑定域名"。
// token: Cloudflare API Token（需 DNS:Edit 权限）；zone: 你的根域名（如 basicgame.cc.cd）；
// sub: 子域名（如 play，留空表示根域名）；target: 目标 IP 或 CNAME 地址；typ: "A" 或 "CNAME"。
func cloudflareUpsertDNS(token, zone, sub, target, typ string) (string, error) {
	token = strings.TrimSpace(token)
	zone = strings.TrimSpace(zone)
	target = strings.TrimSpace(target)
	typ = strings.ToUpper(strings.TrimSpace(typ))
	if token == "" || zone == "" || target == "" {
		return "", fmt.Errorf("请填写 Token、域名与目标")
	}
	if typ != "A" && typ != "CNAME" {
		return "", fmt.Errorf("记录类型仅支持 A 或 CNAME")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	fullName := zone
	if sub != "" {
		fullName = sub + "." + zone
	}

	// 1) 解析 zone id
	zoneID, err := cfGetZoneID(client, token, zone)
	if err != nil {
		return "", err
	}

	// 2) 查询已有记录
	existingID, err := cfFindRecord(client, token, zoneID, fullName)
	if err != nil {
		return "", err
	}

	body := map[string]interface{}{
		"type":    typ,
		"name":    fullName,
		"content": target,
		"ttl":     60,
		"proxied": false,
	}
	payload, _ := json.Marshal(body)

	var method, url string
	if existingID != "" {
		method = "PUT"
		url = "https://api.cloudflare.com/client/v4/zones/" + zoneID + "/dns_records/" + existingID
	} else {
		method = "POST"
		url = "https://api.cloudflare.com/client/v4/zones/" + zoneID + "/dns_records"
	}
	req, _ := http.NewRequest(method, url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(data, &out)
	if !out.Success {
		msg := "Cloudflare 返回错误"
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}
		return "", fmt.Errorf("%s", msg)
	}
	return fullName, nil
}

func cfGetZoneID(client *http.Client, token, zone string) (string, error) {
	url := "https://api.cloudflare.com/client/v4/zones?name=" + zone
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	_ = json.Unmarshal(data, &out)
	if !out.Success || len(out.Result) == 0 {
		return "", fmt.Errorf("未找到域名 %s（请确认 Token 具备该域名的 Zone:Read 权限）", zone)
	}
	return out.Result[0].ID, nil
}

func cfFindRecord(client *http.Client, token, zoneID, name string) (string, error) {
	url := "https://api.cloudflare.com/client/v4/zones/" + zoneID + "/dns_records?name=" + name
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	_ = json.Unmarshal(data, &out)
	if !out.Success {
		return "", nil
	}
	if len(out.Result) > 0 {
		return out.Result[0].ID, nil
	}
	return "", nil
}
