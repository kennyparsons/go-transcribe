#!/bin/bash

# This script builds the gotranscribe binary.
# It assumes that whisper.cpp has been cloned into the project root
# and that its Go bindings have been built (see GEMINI.md).

set -e

echo "Building gotranscribe..."

# Get the absolute path to the project root directory.
# This makes the script runnable from any directory.
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# These flags are necessary to link against the whisper.cpp and ggml static libraries.
CGO_CFLAGS="-I${SCRIPT_DIR}/whisper.cpp/include -I${SCRIPT_DIR}/whisper.cpp/ggml/include" \
CGO_LDFLAGS="-L${SCRIPT_DIR}/whisper.cpp/build_go/src -L${SCRIPT_DIR}/whisper.cpp/build_go/ggml/src -L${SCRIPT_DIR}/whisper.cpp/build_go/ggml/src/ggml-metal -L${SCRIPT_DIR}/whisper.cpp/build_go/ggml/src/ggml-blas" \
go build -o go-transcribe ./cmd/whispcli

echo "✅ Build complete! The binary is located at ./go-transcribe"