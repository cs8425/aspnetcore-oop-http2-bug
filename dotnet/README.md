# Minimal Request Dump API

This is a minimalist ASP.NET Core Web API project designed specifically for **bug reproduction**. 

## Description
The application contains a single handler that captures all incoming request details—including the full path (subpaths) and all headers—and returns them directly in the response body as plain text.

## Purpose
This tool is used to verify how a specific environment or proxy handles request headers and URI routing by "echoing" back exactly what the server receives.

## Prerequisites
*   [.NET SDK](https://dotnet.microsoft.com/download) installed on your machine.

## Build Instructions
To build the project, run the following command in the this directory. This command ensures the output is in English, sets the environment to Development, and specifies the output directory:

```bash
DOTNET_CLI_UI_LANGUAGE=en dotnet publish -c Debug -p:EnvironmentName=Development -p:UseAppHost=false -p:PublishDir="D:/wwwroot/dotnetapi"
```
