#!/usr/bin/env bash
#
# Prepare an Oracle Cloud Always Free instance to run NPDMS.
#
# Handles both images Oracle offers by default:
#   - Oracle Linux 8/9  (user: opc,    package manager: dnf, firewall: firewalld)
#   - Ubuntu 22.04/24.04 (user: ubuntu, package manager: apt, firewall: iptables)
#
# Run it on the instance, not from your laptop:
#   curl -fsSL <this file> -o setup.sh && bash setup.sh
# or, if the repository is already there:
#   bash deploy/oracle-free/setup.sh
#
set -euo pipefail

info() { printf '\033[0;34m›\033[0m %s\n' "$1"; }
ok()   { printf '\033[0;32m✓\033[0m %s\n' "$1"; }
warn() { printf '\033[0;33m!\033[0m %s\n' "$1"; }

[[ $EUID -eq 0 ]] && { echo "Run as the normal user (opc or ubuntu), not root — sudo is used where needed." >&2; exit 1; }

# ------------------------------------------------------------------- distro --
if [[ -f /etc/oracle-release ]] || grep -qi 'oracle' /etc/os-release 2>/dev/null; then
  DISTRO=oracle
  PKG="sudo dnf -y"
elif grep -qi 'ubuntu' /etc/os-release 2>/dev/null; then
  DISTRO=ubuntu
  PKG="sudo apt-get -y"
else
  warn "Unrecognised distribution — assuming Ubuntu-like"
  DISTRO=ubuntu
  PKG="sudo apt-get -y"
fi
ok "Detected: $DISTRO ($(uname -m))"

# --------------------------------------------------------------------- swap --
# The micro shape has 1 GB and no swap. Without it a build or a Postgres
# checkpoint can take the whole box down. This is the single most important
# step on this shape.
if [[ $(swapon --show | wc -l) -gt 0 ]]; then
  ok "Swap already present"
else
  info "Creating a 2 GB swapfile"
  sudo fallocate -l 2G /swapfile 2>/dev/null || sudo dd if=/dev/zero of=/swapfile bs=1M count=2048
  sudo chmod 600 /swapfile
  sudo mkswap /swapfile >/dev/null
  sudo swapon /swapfile
  grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab >/dev/null
  # Prefer RAM but allow swap before the OOM killer gets involved.
  sudo sysctl -q vm.swappiness=20
  grep -q 'vm.swappiness' /etc/sysctl.conf || echo 'vm.swappiness=20' | sudo tee -a /etc/sysctl.conf >/dev/null
  ok "2 GB swap active"
fi

# ------------------------------------------------------------------- docker --
if command -v docker >/dev/null 2>&1; then
  ok "Docker already installed"
else
  info "Installing Docker"
  if [[ $DISTRO == oracle ]]; then
    # Oracle Linux ships podman; docker-ce comes from Docker's own repository.
    $PKG install dnf-utils >/dev/null
    sudo dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo >/dev/null 2>&1 || true
    # Oracle Linux carries podman-docker, which conflicts with docker-ce.
    sudo dnf -y remove podman-docker runc >/dev/null 2>&1 || true
    $PKG install docker-ce docker-ce-cli containerd.io docker-compose-plugin >/dev/null
  else
    $PKG update >/dev/null
    $PKG install ca-certificates curl gnupg >/dev/null
    sudo install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    sudo chmod a+r /etc/apt/keyrings/docker.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
      | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
    $PKG update >/dev/null
    $PKG install docker-ce docker-ce-cli containerd.io docker-compose-plugin >/dev/null
  fi
  sudo systemctl enable --now docker
  sudo usermod -aG docker "$USER"
  ok "Docker installed — log out and back in for group membership to take effect"
fi

# ----------------------------------------------------------------- firewall --
# Oracle images block ports locally even when the VCN Security List allows them.
# Both layers have to be opened; this handles the instance side only.
info "Opening ports on the instance firewall"
if [[ $DISTRO == oracle ]] && command -v firewall-cmd >/dev/null 2>&1; then
  sudo firewall-cmd --permanent --add-service=http  >/dev/null 2>&1 || true
  sudo firewall-cmd --permanent --add-service=https >/dev/null 2>&1 || true
  sudo firewall-cmd --reload >/dev/null 2>&1 || true
  ok "firewalld: 80 and 443 open"
else
  # Oracle's Ubuntu images ship a restrictive INPUT chain; insert before the
  # blanket REJECT rather than appending after it.
  sudo iptables -I INPUT 5 -p tcp --dport 80  -j ACCEPT 2>/dev/null || true
  sudo iptables -I INPUT 6 -p tcp --dport 443 -j ACCEPT 2>/dev/null || true
  if command -v netfilter-persistent >/dev/null 2>&1; then
    sudo netfilter-persistent save >/dev/null 2>&1 || true
  else
    $PKG install iptables-persistent >/dev/null 2>&1 || true
  fi
  ok "iptables: 80 and 443 open"
fi

warn "The VCN Security List or NSG must allow the same ports — the console side is not scriptable from here."

# ------------------------------------------------------------------- report --
echo
ok "Instance prepared."
free -h | sed 's/^/  /'
echo
echo "  Next:"
echo "    cd deploy/oracle-free"
echo "    cp .env.example .env    # fill in every CHANGE_ME"
echo "    docker compose up -d --build"
echo "    ../../scripts/bootstrap-db.sh --with-demo-data"
