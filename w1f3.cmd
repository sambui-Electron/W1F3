@echo off
setlocal
set "W1F3_EXE=%~dp0w1f3.exe"
if not exist "%W1F3_EXE%" (
  echo W1F3 executable was not found: "%W1F3_EXE%"
  exit /b 1
)
"%W1F3_EXE%" %*
