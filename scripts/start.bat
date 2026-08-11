@echo off
setlocal
cd /d "%~dp0"
set "FIRST_RUN=0"
if not exist ".config.yaml" set "FIRST_RUN=1"
echo Starting xiaozhi-server-go...
echo.
xiaozhi-server.exe
echo.
if "%FIRST_RUN%"=="1" (
  echo First start creates .config.yaml. Edit it, then run start.bat again.
)
pause
