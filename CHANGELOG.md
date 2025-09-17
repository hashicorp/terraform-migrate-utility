# v0.0.3 (17th Sep 2025)

## Enhancements
* Added support for listing resource from terraform state file.
* Fixed bug in terraform workspace to stack address mapping function.

# v0.0.2 (25st Aug 2025)

## Enhancements
* Environment variable filtering and error handling.
* The validateStacksFiles function now more robustly checks that the provided path exists and is a directory before running validation, improving reliability and error messaging.
* The stack validation step in WorkspaceToStackAddressMap has been commented out, possibly to temporarily bypass validation during development or testing.

## General Improvements:
* Added the errors package import to support improved error handling throughout the file.…onment variables in stack validation


# v0.0.1 (21st Aug 2025)

## New Features
* RPC API client for Terraform state to stack state transformation.
* Modules and Resources mapping utility for API ingress.
