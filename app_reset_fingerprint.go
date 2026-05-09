package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"windsurf-tools-wails/backend/utils"
)

// ResetMachineFingerprint 重置机器码（不删除聊天记录）
func (a *App) ResetMachineFingerprint() error {
	utils.DLog("[重置] 开始重置机器码...")

	// 1. 删除注册表设备ID（Windows）
	if runtime.GOOS == "windows" {
		if err := resetRegistryDeviceID(); err != nil {
			utils.DLog("[重置] 注册表 deviceid 删除失败: %v", err)
		} else {
			utils.DLog("[重置] ✓ 已删除注册表 deviceid")
		}
	}

	// 2. 删除文件系统设备ID
	if err := resetFileDeviceIDs(); err != nil {
		utils.DLog("[重置] 文件设备ID删除失败: %v", err)
		return fmt.Errorf("重置文件设备ID失败: %w", err)
	}
	utils.DLog("[重置] ✓ 已删除文件系统设备ID")

	// 3. 删除认证状态（但不删除聊天记录）
	if err := resetAuthState(); err != nil {
		utils.DLog("[重置] 认证状态删除失败: %v", err)
	} else {
		utils.DLog("[重置] ✓ 已删除认证状态")
	}

	// 4. 删除会话缓存（限速状态）
	if err := resetSessionCache(); err != nil {
		utils.DLog("[重置] 会话缓存删除失败: %v", err)
	} else {
		utils.DLog("[重置] ✓ 已删除会话缓存")
	}

	utils.DLog("[重置] 机器码重置完成")
	return nil
}

// resetRegistryDeviceID 删除注册表中的设备ID
func resetRegistryDeviceID() error {
	if runtime.GOOS != "windows" {
		return nil
	}

	cmd := exec.Command("reg", "delete", `HKCU\SOFTWARE\Microsoft\DeveloperTools`, "/v", "deviceid", "/f")
	hideWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 忽略"键不存在"的错误
		if strings.Contains(string(output), "找不到") || strings.Contains(string(output), "not find") {
			return nil
		}
		return fmt.Errorf("删除注册表失败: %w, output: %s", err, string(output))
	}
	return nil
}

// resetFileDeviceIDs 删除文件系统中的设备ID
func resetFileDeviceIDs() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}

	// 删除 installation_id
	installationIDPath := filepath.Join(home, ".codeium", "windsurf", "installation_id")
	if err := os.Remove(installationIDPath); err != nil && !os.IsNotExist(err) {
		utils.DLog("[重置] 删除 installation_id 失败: %v", err)
	}

	// 删除 storage.json（包含 machineId, macMachineId, devDeviceId）
	appData := os.Getenv("APPDATA")
	if appData != "" {
		storagePath := filepath.Join(appData, "Windsurf", "storage.json")
		if err := os.Remove(storagePath); err != nil && !os.IsNotExist(err) {
			utils.DLog("[重置] 删除 storage.json 失败: %v", err)
		}
	}

	return nil
}

// resetAuthState 删除认证状态（但保留聊天记录）
func resetAuthState() error {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return nil
	}

	// 删除 windsurf_auth.json
	authPath := filepath.Join(appData, "Windsurf", "User", "globalStorage", "windsurf_auth.json")
	if err := os.Remove(authPath); err != nil && !os.IsNotExist(err) {
		utils.DLog("[重置] 删除 windsurf_auth.json 失败: %v", err)
	}

	// 删除备份文件
	matches, _ := filepath.Glob(authPath + ".bak.*")
	for _, match := range matches {
		os.Remove(match)
	}

	// 删除 codeium config
	home, _ := os.UserHomeDir()
	if home != "" {
		configPath := filepath.Join(home, ".codeium", "config.json")
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			utils.DLog("[重置] 删除 codeium config.json 失败: %v", err)
		}
	}

	return nil
}

// resetSessionCache 删除会话缓存（限速状态存储在这里）
func resetSessionCache() error {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return nil
	}

	// 删除 state.vscdb（限速缓存存储在这里）
	stateDBPath := filepath.Join(appData, "Windsurf", "User", "globalStorage", "state.vscdb")
	if err := os.Remove(stateDBPath); err != nil && !os.IsNotExist(err) {
		utils.DLog("[重置] 删除 state.vscdb 失败: %v", err)
	}

	// 删除备份
	backupPath := stateDBPath + ".backup"
	os.Remove(backupPath)

	return nil
}

// hideWindow 隐藏子进程窗口（Windows）
// 在 exec_windows.go 和 exec_other.go 中定义
