#!/usr/bin/env bash
# Source only in offline test processes, never in application startup scripts.
# Clear credentials, profiles, web identity, container providers and endpoints.
while IFS= read -r aws_test_env_name; do
  unset "$aws_test_env_name"
done < <(compgen -e AWS_)
unset aws_test_env_name
export AWS_CONFIG_FILE=/dev/null
export AWS_SHARED_CREDENTIALS_FILE=/dev/null
export AWS_EC2_METADATA_DISABLED=true
