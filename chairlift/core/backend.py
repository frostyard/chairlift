from enum import Enum

import time
import os
import subprocess
import logging

script_base_path = None

dry_run = True

logger = logging.getLogger("ChairLift::Backend")

_progress_subscribers = []
_error_subscribers = []

errors = []

class ProgressState(Enum):
    Initialized = 1
    Running = 2
    Finished = 3
    Failed = 4



def _install_flatpak(id: str):
    return run_script("flatpak", [id])




def run_script(name: str, args: list[str], root: bool = False, input_data: str = None) -> bool:
    if dry_run:
        print("dry-run", name, args)
        time.sleep(0.3)
        return True
    if script_base_path == None:
        print("Could not run operation", name, args, "due to missing script base path")
        return True
    script_path = os.path.join(script_base_path, name)
    command = [script_path] + args
    if root:
        command = ["pkexec"] + command

    logger.info(f"Executing command: {command}")

    process = subprocess.Popen(
        command,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        stdin=subprocess.PIPE
    )

    result, _ = process.communicate(input=input_data)

    logger.info(f"Output from {name}:\n{result}")

    if process.returncode != 0:
        report_error(name, command, result)
        print(name, args, "returned an error:")
        print(result)
        return False

    return True

def run_script_with_output(name: str, args: list[str], root: bool = False, input_data: str = None) -> tuple[bool, str]:
    """Execute a script and return (success, stdout).

    Mirrors run_script but also returns the captured stdout for callers that need
    to consume script output.
    """
    if dry_run:
        print("dry-run", name, args)
        time.sleep(0.3)
        return True, ""
    if script_base_path == None:
        print("Could not run operation", name, args, "due to missing script base path")
        return True, ""
    script_path = os.path.join(script_base_path, name)
    command = [script_path] + args
    if root:
        command = ["pkexec"] + command

    logger.info(f"Executing command: {command}")

    process = subprocess.Popen(
        command,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        stdin=subprocess.PIPE
    )

    result, _ = process.communicate(input=input_data)

    logger.info(f"Output from {name}:\n{result}")

    if process.returncode != 0:
        report_error(name, command, result)
        print(name, args, "returned an error:")
        print(result)
        return False, result or ""

    return True, result or ""

_error_count = 0
_lock_error_count = False

def report_error(script_name: str, command: list[str], message: str):
    global _error_count
    global _lock_error_count
    while(_lock_error_count):
        time.sleep(0.5)
    _lock_error_count = True

    errors.append(message)

    for callback in _error_subscribers:
        callback(script_name, command, _error_count)

    _error_count = _error_count + 1
    _lock_error_count = False

_deferred_actions = {}



def install_flatpak_deferred(id: str, name: str):
    global _deferred_actions
    action_id = "install_flatpak"
    uid = action_id+id
    action_info = {"app_id": id, "app_name": name}
    def install_flatpak():
        _run_function_with_progress(action_id, uid, action_info, _install_flatpak, id)
    _deferred_actions[uid] = {"action_id": action_id, "callback": install_flatpak, "info": action_info}
    report_progress(action_id, uid, ProgressState.Initialized, action_info)



def _run_function_with_progress(action_id: str, uid: str, action_info: dict, function, *args):
    report_progress(action_id, uid, ProgressState.Running, action_info)
    success = function(*args)
    if not success:
        report_progress(action_id, uid, ProgressState.Failed, action_info)
    else:
        report_progress(action_id, uid, ProgressState.Finished, action_info)

def clear_flatpak_deferred():
    global _deferred_actions
    new_list = {}
    for uid, action in _deferred_actions.items():
        if action["action_id"] != "install_flatpak":
            new_list[uid] = action
    _deferred_actions = new_list

def start_deferred_actions():
    global _deferred_actions
    for _, action in _deferred_actions.items():
        action["callback"]()
    id = "all_actions"
    report_progress(id, id, ProgressState.Finished)

def subscribe_progress(callback):
    global _deferred_actions
    global _progress_subscribers
    _progress_subscribers.append(callback)
    for uid, deferred_action in _deferred_actions.items():
        info = None
        if "info" in deferred_action:
            info = deferred_action["info"]
        callback(deferred_action["action_id"], uid, ProgressState.Initialized, info)

def report_progress(id: str, uid: str, state: ProgressState, info = None):
    global _progress_subscribers
    for subscriber in _progress_subscribers:
        subscriber(id, uid, state, info)

def set_script_path(path: str):
    global script_base_path
    script_base_path = path

def set_dry_run(dry: bool):
    global dry_run
    dry_run = dry

def subscribe_errors(callback):
    global _error_subscribers
    _error_subscribers.append(callback)


def run_script_streaming(name: str, args: list[str], root: bool = False, line_callback=None) -> bool:
    """Execute a script and stream its output line-by-line to a callback.

    This is designed for scripts that output JSON Lines (one JSON object per line)
    for real-time progress monitoring.

    Args:
        name: Script name to execute
        args: Arguments to pass to the script
        root: Whether to run with pkexec for root privileges
        line_callback: Function called with each line of output (str)

    Returns:
        bool: True if script succeeded, False otherwise
    """
    if dry_run:
        print("dry-run (streaming)", name, args)
        # Simulate some progress events for dry-run testing
        import json
        import time
        fake_events = [
            {"type": "message", "message": "Dry run: Downloading update..."},
            {"type": "step", "step": 1, "total_steps": 4, "step_name": "Clearing target partition"},
            {"type": "step", "step": 2, "total_steps": 4, "step_name": "Pulling container image"},
            {"type": "step", "step": 3, "total_steps": 4, "step_name": "Extracting container"},
            {"type": "progress", "percent": 25, "message": "Layer 1/4"},
            {"type": "progress", "percent": 50, "message": "Layer 2/4"},
            {"type": "progress", "percent": 75, "message": "Layer 3/4"},
            {"type": "progress", "percent": 100, "message": "Layer 4/4"},
            {"type": "step", "step": 4, "total_steps": 4, "step_name": "Updating bootloader"},
            {"type": "complete", "message": "Update complete (dry run)"},
        ]
        for event in fake_events:
            if line_callback:
                line_callback(json.dumps(event))
            time.sleep(0.3)
        return True

    if script_base_path is None:
        print("Could not run operation", name, args, "due to missing script base path")
        return False

    script_path = os.path.join(script_base_path, name)
    command = [script_path] + args
    if root:
        command = ["pkexec"] + command

    logger.info(f"Executing streaming command: {command}")

    process = subprocess.Popen(
        command,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        bufsize=1  # Line buffered
    )

    # Read output line by line and pass to callback
    for line in process.stdout:
        line = line.strip()
        if line and line_callback:
            line_callback(line)
        logger.debug(f"Stream line from {name}: {line}")

    process.wait()

    if process.returncode != 0:
        report_error(name, command, f"Script exited with code {process.returncode}")
        print(name, args, "returned an error (exit code", process.returncode, ")")
        return False

    return True