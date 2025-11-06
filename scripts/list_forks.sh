#!/bin/bash

# Script to list all forks of the relab/hotstuff repository
# Uses the GitHub API to fetch fork information

REPO_OWNER="relab"
REPO_NAME="hotstuff"
API_URL="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/forks"

echo "Fetching forks of ${REPO_OWNER}/${REPO_NAME}..."
echo ""

# Check if jq is available for better JSON parsing
if command -v jq &> /dev/null; then
    USE_JQ=true
else
    USE_JQ=false
fi

# Fetch forks from GitHub API
# Use per_page=100 to get up to 100 forks per page
if command -v curl &> /dev/null; then
    response=$(curl -s "${API_URL}?per_page=100&sort=stargazers" 2>&1)
    curl_exit=$?
else
    echo "Error: curl is not installed"
    echo ""
    echo "Alternative ways to view forks:"
    echo "  1. Visit: https://github.com/${REPO_OWNER}/${REPO_NAME}/network/members"
    echo "  2. Use GitHub CLI: gh api repos/${REPO_OWNER}/${REPO_NAME}/forks"
    echo "  3. Check the FORKS.md file in this repository"
    exit 1
fi

# Check if curl was successful
if [ $curl_exit -ne 0 ] || echo "$response" | grep -q "Blocked by"; then
    echo "Unable to fetch data from GitHub API (network may be restricted)"
    echo ""
    echo "Alternative ways to view forks:"
    echo "  1. Visit: https://github.com/${REPO_OWNER}/${REPO_NAME}/network/members"
    echo "  2. Use GitHub CLI: gh api repos/${REPO_OWNER}/${REPO_NAME}/forks"
    echo "  3. Check the FORKS.md file in this repository"
    exit 0
fi

# Check if response contains an error
if echo "$response" | grep -q "\"message\""; then
    echo "Error from GitHub API:"
    echo "$response" | grep -o '"message":"[^"]*"'
    exit 1
fi

# Parse fork information
if [ "$USE_JQ" = true ]; then
    # Use jq for better parsing
    total_forks=$(echo "$response" | jq '. | length')
    
    if [ "$total_forks" -eq 0 ]; then
        echo "No forks found for this repository."
        exit 0
    fi
    
    echo "Found ${total_forks} fork(s):"
    echo ""
    
    echo "$response" | jq -r '.[] | "  - \(.full_name)\n    Owner: \(.owner.login)\n    URL: \(.html_url)\n    Stars: \(.stargazers_count)\n    Updated: \(.updated_at)\n"'
else
    # Fallback to grep/sed parsing
    total_forks=$(echo "$response" | grep -o '"full_name"' | wc -l)
    
    if [ "$total_forks" -eq 0 ]; then
        echo "No forks found for this repository."
        exit 0
    fi
    
    echo "Found ${total_forks} fork(s):"
    echo ""
    
    # Parse and display fork information
    echo "$response" | grep -o '"full_name":"[^"]*"' | sed 's/"full_name":"//;s/"$//' | while read -r fork_name; do
        echo "  - https://github.com/${fork_name}"
    done
fi

echo ""
echo "Done!"
