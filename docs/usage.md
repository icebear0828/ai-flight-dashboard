# AI Flight Dashboard 使用手册

AI Flight Dashboard 是一个极简、零依赖的 AI 资产终端飞行仪表盘。它通过被动雷达技术无侵入式监听你本地或服务器上的 AI CLI 工具消耗。

## 基础使用

### 1. 终端 TUI 模式 (极客 HUD)
在你的开发终端侧边栏或 Tmux 的一个小分屏中运行：
```bash
./dashboard
```
即可实时看到 Token 消耗跳动。它会在本地 `stats/usage.db` 保存所有的消耗记录。

### 2. Web 看板模式
以 Web 模式运行，你可以通过浏览器访问丰富的数据报表：
```bash
./dashboard --web --port 9100
```
访问 `http://localhost:9100` 查看。

---

## 高级特性：远程集群监控 (Remote Telemetry)

如果你有多台服务器或多台开发机，你可以让其中一台运行 Web 面板作为主控，其他机器作为 Forwarder 探针将日志实时推送上来，免去本地数据库的同步麻烦。

### 一键部署集群 (小白推荐)

为简化配置步骤，项目提供了 `scripts/deploy.sh` 交互式部署脚本：

```bash
chmod +x ./scripts/deploy.sh
sudo ./scripts/deploy.sh
```

运行后，脚本会提问你是配置为**主控端 (Receiver)** 还是 **探针端 (Forwarder)**，只需要输入通信密钥 (Token) 和对应地址，脚本将自动编译并把它配置为开机自启的 Systemd 后台服务。

### 手动部署集群

#### 第一步：启动主控服务端 (Receiver)

在作为中心看板的机器（或你的主 Mac）上启动带 Token 校验的 Web 模式：
```bash
# --token 设置一个通信密钥，防止接口被恶意访问
./dashboard --web --port 9100 --token "your-secret-key"
```

### 第二步：在其他机器启动探针 (Forwarder)

将 `dashboard` 二进制文件复制到你的其他服务器上。
使用 `--forward-to` 参数指向主控机器的 `/api/track` 接口：

```bash
# --device-id 标识该服务器的名称
# --forward-to 填写主控板的接口地址
./dashboard \
  --device-id "ubuntu-server-1" \
  --forward-to "http://YOUR_MAIN_IP:9100/api/track" \
  --token "your-secret-key"
```

启动后，该节点将**不会**生成本地数据库，而是将监听到的 Claude Code / Gemini CLI 的所有 Token 消耗**实时转发**到主控看板。你可以在主控板 Web 界面上选择设备过滤，查看集群总览或单台服务器的开销。

---

## 数据管理：导出、导入与去重

### 导出 (Export)
将本地数据库导出为 CSV 格式，默认只导出当前设备的数据：
```bash
# 导出当前设备数据到文件
./dashboard export > usage.csv

# 导出所有设备数据
./dashboard --device-id all export > all_usage.csv

# 导出指定设备
./dashboard --device-id "ubuntu-server-1" export > server1.csv
```

### 导入 (Import)
从 CSV 文件导入数据到本地数据库，自动去重（已有的记录会被跳过）：
```bash
./dashboard import server1.csv
# ✅ Import complete: 1234 imported, 56 skipped (duplicates)
```

### 去重 (Dedup)
清理数据库中的重复记录（保留最早的那条）：
```bash
./dashboard dedup
# ✅ Removed 23493 duplicate records
```

### 典型场景：离线同步多台设备

如果你的设备之间没有网络连接（无法使用 Forwarder），可以用导出/导入实现数据合并：
```bash
# 在设备 A 上导出
./dashboard export > device_a.csv

# 将 CSV 拷贝到主控机（U盘、scp 等任意方式）
scp device_a.csv user@main-server:~/

# 在主控机上导入（自动去重，可重复执行）
./dashboard import device_a.csv
```

