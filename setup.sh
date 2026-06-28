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

echo -e "${BLUE}[3/3] Building calctl...${NC}"
if go build -o calctl .; then
    echo -e "${GREEN}✔ calctl built successfully.${NC}"
else
    echo -e "${RED}Build failed.${NC}"
    exit 1
fi
echo ""

echo -e "${BLUE}${BOLD}Global installation:${NC}"
read -p "Copy calctl to /usr/local/bin? (y/n): " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    if sudo cp calctl /usr/local/bin/calctl; then
        echo -e "${GREEN}✔ Installed globally. Run: calctl --help${NC}"
    else
        echo -e "${YELLOW}Run locally: ./calctl --help${NC}"
    fi
else
    echo -e "${YELLOW}Run locally: ./calctl --help${NC}"
fi

echo ""
echo -e "${GREEN}${BOLD}Done! Try:${NC}"
echo -e "  ${YELLOW}calctl sync${NC}           — sync Apple Calendar"
echo -e "  ${YELLOW}calctl list --today${NC}   — show today's events"
echo -e "  ${YELLOW}calctl free --next 7${NC}  — find free slots this week"
