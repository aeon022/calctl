#!/bin/bash
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BLUE}${BOLD}====================================${NC}"
echo -e "${BLUE}${BOLD}      calctl Setup & Installation   ${NC}"
echo -e "${BLUE}${BOLD}====================================${NC}"
echo ""

echo -e "${BLUE}[1/3] Checking Go installation...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed.${NC}"
    echo -e "Install via Homebrew: ${YELLOW}brew install go${NC}"
    exit 1
fi
echo -e "${GREEN}✔ $(go version)${NC}"
echo ""

echo -e "${BLUE}[2/3] Downloading dependencies...${NC}"
if go mod download; then
    echo -e "${GREEN}✔ Dependencies ready.${NC}"
else
    echo -e "${RED}Failed to download dependencies.${NC}"
    exit 1
fi
echo ""

echo -e "${BLUE}[3/3] Building and installing calctl...${NC}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$INSTALL_DIR"
if go build -o "$INSTALL_DIR/calctl" .; then
    echo -e "${GREEN}✔ calctl installed to $INSTALL_DIR/calctl${NC}"
else
    echo -e "${RED}Build failed.${NC}"
    exit 1
fi

echo ""

# ── MCP: register in ~/.claude.json ───────────────────────────────────────────
CLAUDE_JSON="$HOME/.claude.json"
if command -v python3 &>/dev/null; then
  python3 - "$CLAUDE_JSON" "$INSTALL_DIR/calctl" <<'PYEOF'
import json, sys, os

claude_json = sys.argv[1]
binary_path = sys.argv[2]

data = {}
if os.path.exists(claude_json):
    with open(claude_json) as f:
        try:
            data = json.load(f)
        except Exception:
            pass

data.setdefault("mcpServers", {})
data["mcpServers"]["calctl"] = {
    "command": binary_path,
    "args": ["mcp"]
}

with open(claude_json, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")

print("✓ MCP server registered in ~/.claude.json")
print("  Restart Claude Code to activate calctl MCP tools")
PYEOF
else
  echo "  To enable MCP (Claude Code integration), add to ~/.claude.json:"
  echo "  \"mcpServers\": { \"calctl\": { \"command\": \"$INSTALL_DIR/calctl\", \"args\": [\"mcp\"] } }"
fi

echo ""
echo -e "${GREEN}${BOLD}Done! Try:${NC}"
echo -e "  ${YELLOW}calctl sync${NC}           — sync Apple Calendar"
echo -e "  ${YELLOW}calctl list --today${NC}   — show today's events"
echo -e "  ${YELLOW}calctl free --next 7${NC}  — find free slots this week"
