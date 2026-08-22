# 安装 — Linux（apt）

Spotter 1.0 起提供官方 apt 仓库。

```bash
# 1. 添加 GPG key
curl -fsSL https://apt.spotter.dev/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/spotter.gpg

# 2. 添加仓库
echo "deb [signed-by=/usr/share/keyrings/spotter.gpg] https://apt.spotter.dev stable main" \
  | sudo tee /etc/apt/sources.list.d/spotter.list

# 3. 安装
sudo apt update
sudo apt install spotter-client    # GUI
sudo apt install spotterd           # 设备端守护

# 4. 启动设备端
sudo systemctl enable --now spotterd
```

升级：

```bash
sudo apt update && sudo apt upgrade
```

源码安装：见 `docs/install-from-source.md`。
