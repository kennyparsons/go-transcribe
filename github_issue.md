### Subject: Build Failure on macOS CI: `i8mm` feature error despite attempting to disable it

Hello `whisper.cpp` team,

First, thank you for this incredible project. I'm using the Go bindings in a CLI tool and have run into a persistent build issue specifically within the GitHub Actions `macos-latest` environment.

### What I'm Doing
I am building the Go bindings as part of a CI pipeline using a `macos-latest` runner. The workflow checks out the repository (with `whisper.cpp` as a submodule) and runs `make` from the `bindings/go` directory.

### The Error
The build consistently fails during the C/C++ compilation of the submodule with the following error:
```
/Users/runner/work/go-transcribe/go-transcribe/whisper.cpp/ggml/src/ggml-cpu/arch/arm/quants.c:217:88: error: always_inline function 'vmmlaq_s32' requires target feature 'i8mm', but would be inlined into function 'ggml_vec_dot_q4_0_q8_0' that is compiled without support for 'i8mm'
```

### Environment
*   **CI Runner:** `macos-latest` (ARM64/Apple Silicon architecture)
*   **whisper.cpp:** Latest commit from the `master` branch as of this week.

### What I've Tried
Based on research from similar issues (#1135, #1121), the problem seems to be that the Clang compiler on the runner is incorrectly trying to use the `i8mm` instruction set. I have tried several methods to explicitly disable this feature, but the error persists.

Here are the steps I've taken:

1.  **Using the Environment Variable:** I tried setting the recommended environment variable directly before the `make` command in my GitHub Actions workflow:
    ```yaml
    - name: Build whisper.cpp bindings
      run: WHISPER_NO_I8MM=1 make -C whisper.cpp/bindings/go
    ```

2.  **Passing a `CMake` Flag:** I attempted to pass the flag directly to `CMake` via the `make` command, as this seems to be a more direct way to influence the C++ build:
    ```yaml
    - name: Build whisper.cpp bindings
      run: make -C whisper.cpp/bindings/go CMAKE_C_FLAGS="-DWHISPER_NO_I8MM=1"
    ```

3.  **GoReleaser Configuration:** My final release binary is built with GoReleaser. I have also tried to disable the feature there by setting the environment variable and by passing the flag via `CGO_CFLAGS`:
    ```yaml
    # Attempt A
    env:
      - WHISPER_NO_I8MM=1
    
    # Attempt B
    env:
      - CGO_CFLAGS=-DWHISPER_NO_I8MM
    ```

Unfortunately, none of these approaches have resolved the error on the GitHub runner.

### My Question
Could you please advise on the correct and most reliable method to disable the `i8mm` feature set when building the Go bindings within a GitHub Actions `macos-latest` environment? It seems the standard methods are not being respected by the build chain, and I'm not sure what to try next.

Thank you for your time and any help you can provide!
