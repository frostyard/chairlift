set dotenv-load := true


default:
    @just --list --unsorted

setup:
    @echo "Setting up development environment..."
    distrobox assemble create
    distrobox enter chairlift 

enter:
    @distrobox enter chairlift

build:
    @echo "Building the project..."
    meson setup --reconfigure build
    meson compile -C build

local:
    @echo "building to local..."
    meson setup --prefix="$(pwd)/install" build
    meson compile -C build
    meson install -C build

clean:
    @echo "Cleaning build artifacts..."
    rm -rf build


run:
    @echo "Running the application..."
    python3 test.py -d 

pot:
    @echo "Generating translation template..."
    meson compile -C build chairlift-pot