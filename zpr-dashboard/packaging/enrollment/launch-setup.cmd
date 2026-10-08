@echo off
setlocal
if not exist "%~dp0setup.json" (
    echo An administrator must configure setup.json before enrollment.
    echo See "%~dp0README.txt". No enrollment request was sent.
    pause
    exit /b 1
)
"%~dp0zpr-enrollment-setup.exe" -config "%~dp0setup.json" -user-state -open-browser
if errorlevel 1 (
    echo Setup failed. Keep the saved key and contact your administrator.
    pause
    exit /b 1
)
