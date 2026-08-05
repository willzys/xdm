# Security Policy

## Supported versions

The xdm project has not published a stable release yet.

Security fixes are applied to the latest commit on the `main` branch. Older
commits, development branches, and unofficial builds are not supported.

## Reporting a vulnerability

Do not report security vulnerabilities in public GitHub issues, discussions, or
pull requests.

Send a private report to:

```text
willyz@outlook.com.br
```

Use the subject:

```text
[xdm security] Short vulnerability description
```

Include as much of the following information as possible:

- affected commit, branch, or version;
- affected operating system;
- vulnerability description;
- reproduction steps or proof of concept;
- expected security impact;
- suggested mitigation, if known.

Do not include real OAuth tokens, browser cookies, Direct Message content, or
credentials. Use redacted or test data whenever possible.

## Response process

The maintainer will make a best effort to:

1. acknowledge the report within seven days;
2. validate the issue and determine its severity;
3. coordinate a fix and disclosure timeline with the reporter;
4. publish remediation information after affected users can update.

Response time may vary because xdm is currently maintained on a best-effort
basis.

## Scope

Security reports are especially relevant when they involve:

- authentication flows, including OAuth authorization and callback validation;
- access or refresh token storage;
- operating system keyring integration;
- disclosure of cached Direct Messages;
- unintended credential transmission;
- command execution or path handling;
- dependency vulnerabilities with a demonstrated impact on xdm.

Vulnerabilities affecting the X platform or the X API itself should be reported
through the appropriate X security channels rather than this project.

## Responsible disclosure

Give the maintainer a reasonable opportunity to investigate and release a fix
before publishing vulnerability details.

Good-faith research that avoids privacy violations, data destruction, service
disruption, and access to other users' accounts is appreciated.
