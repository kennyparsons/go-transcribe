# go-transcribe

A command-line tool to transcribe audio and video files using whisper.cpp.

## Installation

### 1. Install Homebrew

Homebrew is required to manage and install required packages. Run the official install script in a terminal window:
```sh
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```


After installation, follow any instructions to add Homebrew to your PATH. 

> Note, you may be asked to install xcode utilities. You can safely allow this. The xcode utilities might take a while to install, but it never takes as long as the estimate shown.

### Step 2: Tap the Repository
Add the go-transcribe repository to Homebrew:


```sh
brew tap kennyparsons/go-transcribe
```

### Step 3: Install Dependencies

Install dependencies and then the go-transcribe tool:
````sh
brew install --cask vlc && \
brew install go-transcribe
```

> This step can take a while.

### Step 4: Verify Installation

Confirm everything installed correctly:
```sh
transcribe version
```

## Usage Guide for go-transcribe

Once installed, you can use the transcribe command. The most common commands are: 


#### Check the Version

Run the following to display the installed version:
```sh
transcribe version
```

#### Setup Wizard

Use the setup command to open the configuration interface. This will start a wizard to help you download transcription models and auto create a config file:
```sh
transcribe setup
```

#### Transcribe a file
Simply pass the path of the file to the transcribe tool. A `txt` file will be output next to the original media file. 

```sh
transcribe /path/to/your/mdeiafile.mp4
```

#### Overriding the Default Model

You can use the `--model` flag to specify a different transcription model for a single command, overriding the configured default. This is useful for transcribing content in other languages, like Japanese.

Example using the Japanese model:
```sh
transcribe --model ~/.config/whisper-cpp/models/ggml-large-v3-kotoba.ja_JP.bin /path/to/your/video.mp4
```
