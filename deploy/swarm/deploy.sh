#!/bin/bash
set -e

# ==============================================================================
# Airoute Docker Swarm Multi-Node Deployment Helper
# ==============================================================================

STACK_NAME="nano-stack"

echo "=========================================================="
echo " Airoute Enterprise Docker Swarm Deployment"
echo "=========================================================="

# Check if Docker Swarm is active
if ! docker info --format '{{.Swarm.LocalNodeState}}' | grep -q "active"; then
    echo "[!] Docker Swarm is not initialized on this host."
    echo "[*] Initializing Docker Swarm..."
    docker swarm init || true
fi

# Ensure overlay network exists
if ! docker network ls --filter name=^nano-swarm-net$ --format '{{.Name}}' | grep -q "nano-swarm-net"; then
    echo "[*] Creating overlay network: nano-swarm-net..."
    docker network create --driver overlay --attachable nano-swarm-net
fi

# Build local gateway image if not present or requested
if [ "$1" == "--build" ] || ! docker image inspect airoute:latest >/dev/null 2>&1; then
    echo "[*] Building airoute:latest production image..."
    docker build -t airoute:latest .
fi

echo "[*] Deploying stack: ${STACK_NAME}..."
docker stack deploy -c docker-stack.yml "${STACK_NAME}"

echo ""
echo "[✓] Deployment command submitted to Docker Swarm!"
echo "[*] To inspect cluster services:"
echo "    docker stack services ${STACK_NAME}"
echo ""
echo "[*] To inspect task allocation across Swarm nodes:"
echo "    docker stack ps ${STACK_NAME}"
echo ""
echo "[*] To follow cluster logs (with distributed TraceID):"
echo "    docker service logs -f ${STACK_NAME}_airoute"
echo ""
echo "[*] Web 工作台 & API are available at:"
echo "    http://<swarm-node-ip>:8080/app/ (或 http://<swarm-node-ip>:8080/)"
echo "=========================================================="
