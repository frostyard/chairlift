#!/bin/bash
# Wrapper script to launch ChairLift with Homebrew environment

# Set up Homebrew environment
BREW_PATH="/home/linuxbrew/.linuxbrew/bin/brew"
if [ -f "$BREW_PATH" ]; then
    eval "$($BREW_PATH shellenv)"
fi

# Launch the application
exec @bindir@/chairlift "$@"
