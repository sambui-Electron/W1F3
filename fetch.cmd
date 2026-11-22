@echo off
setlocal
set "FETCH_EXE=%~dp0fetch.exe"
if not exist "%FETCH_EXE%" (
  echo Fetch executable was not found: "%FETCH_EXE%"
  exit /b 1
)
"%FETCH_EXE%" %*
