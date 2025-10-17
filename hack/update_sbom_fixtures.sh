#!/usr/bin/env sh

set -e

# Function to display usage
usage() {
    echo "Usage: $0 <directory>"
    echo "  Updates all SPDX SBOM fixtures (*.spdx.json) in the directory using trivy"
    exit 1
}

# Check if directory argument is provided
if [ $# -ne 1 ]; then
    usage
fi

DIR="$1"

# Validate directory exists
if [ ! -d "$DIR" ]; then
    echo "Error: Directory '$DIR' does not exist"
    exit 1
fi

# Check if jq is installed
if ! command -v jq >/dev/null 2>&1; then
    echo "Error: jq is required but not installed"
    echo "Install it with: apt-get install jq OR brew install jq"
    exit 1
fi

# Check if trivy is installed
if ! command -v trivy >/dev/null 2>&1; then
    echo "Error: trivy is required but not installed"
    echo "Visit: https://aquasecurity.github.io/trivy/latest/getting-started/installation/"
    exit 1
fi

# Count files first
count=0
for file in "$DIR"/*.spdx.json; do
    if [ -f "$file" ]; then
        count=$((count + 1))
    fi
done

if [ "$count" -eq 0 ]; then
    echo "No *.spdx.json files found in $DIR"
    exit 0
fi

echo "Found $count SPDX files to update"

# Process each file
for file in "$DIR"/*.spdx.json; do
    # Skip if no files match (glob didn't expand)
    [ -f "$file" ] || continue
    
    echo "Processing: $file"
    
    # Extract image name from the "name" field
    image_name=$(jq -r '.name' "$file")
    
    if [ "$image_name" = "null" ] || [ -z "$image_name" ]; then
        echo "  Warning: No image name found in $file, skipping..."
        continue
    fi
    
    echo "  Image: $image_name"
    echo "  Running trivy..."
    
    if trivy image --format spdx-json --output "$file" "$image_name"; then
        echo "  ✓ Successfully updated SBOM for $image_name"
    else
        echo "  ✗ Failed to update SBOM for $image_name"
    fi
    
    echo ""
done

echo "SBOMs update complete!"
