#!/usr/bin/env python3
"""
Generate Open Horizon deployment policy from EdgeLake configuration file.
Converts dotenv format to OpenHorizon deployment policy JSON format.

Usage:
    python create_deployment_policy.py <version> <config_file>

Example:
    python create_deployment_policy.py 1.3.5 configurations/operator_production.env
"""

import argparse
import ast
import json
import os

try:
    FileNotFoundError
except NameError:
    FileNotFoundError = IOError

ROOT_DIR = os.path.dirname(os.path.expanduser(os.path.expandvars(__file__)))
FILE_PATH = os.path.join(ROOT_DIR, 'service.deployment.json')

# Environment variables to ignore (Docker-specific, not needed in OpenHorizon deployment)
IGNORE_LIST = [
    "REMOTE_CLI",
    "ENABLE_NEBULA",
    "NEBULA_NEW_KEYS",
    "IS_LIGHTHOUSE",
    "CIDR_OVERLAY_ADDRESS",
    "LIGHTHOUSE_IP",
    "LIGHTHOUSE_NODE_IP",
    "NIC_TYPE"
]

IS_BASE = True
BASE_POLICY = {
  "label": "${SERVICE_NAME} Deployment Policy",
  "description": "Policy to auto deploy ${SERVICE_NAME}",
  "service": {
    "name": "${SERVICE_NAME}",
    "org": "${HZN_ORG_ID}",
    "arch": "*",
    "serviceVersions": [
      {
        "version": "${SERVICE_VERSION}",
        "priority": {
          "priority_value": 2,
          "retries": 2,
          "retry_durations": 1800
        }
      }
    ]
  },
  "properties": [],
  "constraints": [
    "purpose == edgelake",
    "openhorizon.allowPrivileged == true"
  ],
  "userInput": [
    {
      "serviceOrgid": "${HZN_ORG_ID}",
      "serviceUrl": "${SERVICE_NAME}",
      "serviceVersionRange": "[0.0.0,INFINITY)",
      "inputs": []
    }
  ]
}

# Read existing deployment policy if it exists
if os.path.isfile(FILE_PATH):
    try:
        with open(FILE_PATH, 'r') as f:
            BASE_POLICY = json.load(f)
    except:
        pass
    else:
        IS_BASE = False


def read_file(file_path):
    """
    Read EdgeLake configuration file (dotenv format) and convert to OH userInput format.

    Args:
        file_path (str): Path to the configuration file

    Returns:
        list: List of userInput objects for OpenHorizon deployment policy

    UserInput object format:
        {
          "name": "NODE_TYPE",
          "label": "Description from comment",
          "type": "string|int|bool",
          "value": "operator"
        }
    """
    user_input = []
    key = ""
    description = ""
    value = ""

    with open(file_path, 'r') as fname:
        for line in fname:
            if line.strip() == "":  # Ignore empty lines
                continue
            elif line.startswith('#'):  # Extract description from comments
                description = line.split('#')[-1].strip()
            elif not line.startswith('#'):  # Process key=value pairs
                key, value = line.split("=", 1)
                key = key.strip()

                if key not in IGNORE_LIST:
                    value = value.strip()

                    # Try to evaluate as Python literal (int, bool, etc.)
                    try:
                        value = ast.literal_eval(value)
                    except Exception:
                        pass

                    # Convert string "true"/"false" to boolean
                    if isinstance(value, str) and value.lower() in ['true', 'false']:
                        value = True if value.lower() == 'true' else False

                    # Add to userInput if we have all required fields
                    if key and value and description and value != '""' and value != '':
                        user_input.append({
                            "name": key,
                            "label": description,
                            "type": 'bool' if isinstance(value, bool) else 'int' if isinstance(value, int) else 'string',
                            "value": value
                        })
                        key = ""
                        description = ""
                        value = ""

    return user_input


def main():
    """
    Main function to generate deployment policy from configuration file.
    """
    global BASE_POLICY
    global IS_BASE

    parse = argparse.ArgumentParser(
        description='Generate Open Horizon deployment policy from EdgeLake config file'
    )
    parse.add_argument('version', type=str, help="EdgeLake service version (e.g., 1.3.5)")
    parse.add_argument('config_file', type=str, help='Path to EdgeLake configuration file')
    args = parse.parse_args()

    full_path = os.path.expanduser(os.path.expandvars(args.config_file))
    if not os.path.isfile(full_path):
        raise FileNotFoundError(f"Configuration file not found: {full_path}")

    # Update version only if we're using the base policy
    if IS_BASE is True:
        BASE_POLICY['service']['serviceVersions'][0]['version'] = args.version

    # Read configuration and populate userInput
    BASE_POLICY['userInput'][0]['inputs'] = read_file(file_path=full_path)

    # Write deployment policy to file
    with open(FILE_PATH, 'w') as fname:
        json.dump(BASE_POLICY, fname, indent=2, ensure_ascii=False)

    print(f"✓ Deployment policy generated: {FILE_PATH}")
    print(f"  Version: {args.version}")
    print(f"  Config: {full_path}")
    print(f"  User inputs: {len(BASE_POLICY['userInput'][0]['inputs'])} parameters")


if __name__ == '__main__':
    main()
