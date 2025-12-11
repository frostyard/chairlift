"""
Homebrew package manager interface.

This module provides functions to interact with Homebrew package manager,
including listing installed packages, checking for outdated packages,
and searching for available formulae.
"""

import subprocess
import json
from typing import List, Dict, Optional, Any


class HomebrewError(Exception):
    """Base exception for Homebrew-related errors."""
    pass


class HomebrewNotFoundError(HomebrewError):
    """Raised when Homebrew is not installed or not found in PATH."""
    pass


def _run_brew_command(args: List[str], timeout: int = 30) -> str:
    """
    Run a brew command and return the output.
    
    Args:
        args: List of command arguments (e.g., ['list', '--formula'])
        timeout: Command timeout in seconds
        
    Returns:
        Command output as string
        
    Raises:
        HomebrewNotFoundError: If brew is not found
        HomebrewError: If command fails
    """
    try:
        result = subprocess.run(
            ['brew'] + args,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=True
        )
        return result.stdout
    except FileNotFoundError:
        raise HomebrewNotFoundError("Homebrew not found. Please install Homebrew first.")
    except subprocess.TimeoutExpired:
        raise HomebrewError(f"Command 'brew {' '.join(args)}' timed out after {timeout} seconds")
    except subprocess.CalledProcessError as e:
        raise HomebrewError(f"Brew command failed: {e.stderr}")


def is_homebrew_installed() -> bool:
    """
    Check if Homebrew is installed and accessible.
    
    Returns:
        True if Homebrew is installed, False otherwise
    """
    try:
        subprocess.run(
            ['brew', '--version'],
            capture_output=True,
            timeout=5,
            check=True
        )
        return True
    except (FileNotFoundError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        return False


def list_installed_packages(formula_only: bool = True) -> List[Dict[str, Any]]:
    """
    List all installed Homebrew packages.
    
    Args:
        formula_only: If True, only list formulae (default). If False, include casks too.
        
    Returns:
        List of dictionaries containing package information with keys:
        - name: Package name
        - version: Installed version
        - installed_on_request: Whether explicitly installed by user
        
    Raises:
        HomebrewNotFoundError: If Homebrew is not installed
        HomebrewError: If the command fails
    """
    args = ['info', '--installed', '--json=v2']
    if formula_only:
        args.append('--formula')
    
    output = _run_brew_command(args)
    
    try:
        data = json.loads(output)
        packages = []
        
        # The JSON structure has 'formulae' and 'casks' keys
        items = []
        if formula_only:
            items = data.get('formulae', [])
        else:
            items = data.get('formulae', []) + data.get('casks', [])
        
        for item in items:
            # Get the installed version
            installed_versions = item.get('installed', [])
            version = ''
            if installed_versions:
                if isinstance(installed_versions[0], dict):
                    version = installed_versions[0].get('version', '')
                else:
                    version = str(installed_versions[0])
            
            # Ensure name is a string, not a list
            name = item.get('name', '')
            if isinstance(name, list):
                name = name[0] if name else ''
            
            packages.append({
                'name': str(name),
                'version': version,
                'installed_on_request': item.get('installed_on_request', False)
            })
        return packages
    except json.JSONDecodeError as e:
        raise HomebrewError(f"Failed to parse brew output: {e}")


def list_outdated_packages(formula_only: bool = True) -> List[Dict[str, Any]]:
    """
    List outdated Homebrew packages that have updates available.
    
    Args:
        formula_only: If True, only list formulae (default). If False, include casks too.
        
    Returns:
        List of dictionaries containing outdated package information with keys:
        - name: Package name
        - current_version: Currently installed version
        - latest_version: Latest available version
        - pinned: Whether the package is pinned
        
    Raises:
        HomebrewNotFoundError: If Homebrew is not installed
        HomebrewError: If the command fails
    """
    args = ['outdated', '--json=v2']
    if formula_only:
        args.append('--formula')
    
    output = _run_brew_command(args)
    
    try:
        data = json.loads(output)
        outdated = []
        
        # Handle both formulae and casks
        items = data.get('formulae', [])
        if not formula_only:
            items.extend(data.get('casks', []))
        
        for item in items:
            outdated.append({
                'name': item.get('name', ''),
                'current_version': item.get('installed_versions', [''])[0] if item.get('installed_versions') else '',
                'latest_version': item.get('current_version', ''),
                'pinned': item.get('pinned', False)
            })
        return outdated
    except json.JSONDecodeError as e:
        raise HomebrewError(f"Failed to parse brew output: {e}")


def search_formula(query: str, limit: Optional[int] = None) -> List[Dict[str, str]]:
    """
    Search for available Homebrew formulae.
    
    Args:
        query: Search query string
        limit: Maximum number of results to return (None for all)
        
    Returns:
        List of dictionaries containing search results with keys:
        - name: Formula name
        - description: Formula description
        
    Raises:
        HomebrewNotFoundError: If Homebrew is not installed
        HomebrewError: If the command fails
    """
    output = _run_brew_command(['search', '--formula', '--json', query])
    
    try:
        data = json.loads(output)
        results = []
        
        # Get formula names from search results
        formulae = data.get('formulae', [])
        
        for formula in formulae[:limit] if limit else formulae:
            # Get detailed info for each formula
            try:
                info_output = _run_brew_command(['info', '--json=v2', formula.get('name', '')])
                info_data = json.loads(info_output)
                formula_info = info_data.get('formulae', [{}])[0]
                
                results.append({
                    'name': formula_info.get('name', formula.get('name', '')),
                    'description': formula_info.get('desc', '')
                })
            except (HomebrewError, json.JSONDecodeError):
                # If detailed info fails, use basic info
                results.append({
                    'name': formula.get('name', ''),
                    'description': ''
                })
        
        return results
    except json.JSONDecodeError as e:
        raise HomebrewError(f"Failed to parse brew output: {e}")


def get_package_info(package_name: str) -> Optional[Dict[str, Any]]:
    """
    Get detailed information about a specific package.
    
    Args:
        package_name: Name of the package
        
    Returns:
        Dictionary containing package information with keys:
        - name: Package name
        - version: Latest version
        - description: Package description
        - homepage: Homepage URL
        - installed: Whether currently installed
        - installed_version: Installed version (if installed)
        
    Raises:
        HomebrewNotFoundError: If Homebrew is not installed
        HomebrewError: If the command fails
    """
    output = _run_brew_command(['info', '--json=v2', package_name])
    
    try:
        data = json.loads(output)
        formulae = data.get('formulae', [])
        
        if not formulae:
            return None
        
        formula = formulae[0]
        installed_info = formula.get('installed', [])
        
        return {
            'name': formula.get('name', ''),
            'version': formula.get('versions', {}).get('stable', ''),
            'description': formula.get('desc', ''),
            'homepage': formula.get('homepage', ''),
            'installed': len(installed_info) > 0,
            'installed_version': installed_info[0].get('version', '') if installed_info else None
        }
    except json.JSONDecodeError as e:
        raise HomebrewError(f"Failed to parse brew output: {e}")


def update_homebrew() -> bool:
    """
    Update Homebrew itself and all formulae definitions.
    
    Returns:
        True if update was successful
        
    Raises:
        HomebrewNotFoundError: If Homebrew is not installed
        HomebrewError: If the command fails
    """
    _run_brew_command(['update'], timeout=120)
    return True


def upgrade_package(package_name: Optional[str] = None) -> bool:
    """
    Upgrade a specific package or all outdated packages.
    
    Args:
        package_name: Name of package to upgrade (None to upgrade all)
        
    Returns:
        True if upgrade was successful
        
    Raises:
        HomebrewNotFoundError: If Homebrew is not installed
        HomebrewError: If the command fails
    """
    args = ['upgrade']
    if package_name:
        args.append(package_name)
    
    _run_brew_command(args, timeout=300)
    return True
