@echo off
rem Switch the console to UTF-8; otherwise this file displays as garbage in a GBK terminal.
chcp 65001 >nul 2>&1
rem ARTEX supervisor startup script (Windows)
rem
rem Usage:
rem   start.bat                  Run in the foreground (Ctrl-C to stop)
rem   start.bat -addr :9000      Extra arguments are passed through to artex
rem
rem It does one thing only: run artex.exe, then decide from the exit code whether
rem to start it again.
rem
rem   0      User stopped normally  -> exit the loop
rem   75     Program requested restart -> rerun immediately (page clicked "Update" or "Rollback")
rem   other  Crash                 -> back off and rerun (1->2->4… up to 60 seconds)
rem
rem Downloading, SHA256 verification, and swapping are not done here; artex does
rem it all itself at startup (the selfupdate package). The script stays simple;
rem see the top of start.sh for details.

setlocal enabledelayedexpansion
cd /d "%~dp0"

set "BIN=artex.exe"
if not exist "%BIN%" (
	echo [artex] executable not found: %BIN% 1>&2
	exit /b 1
)

set "RESTART_CODE=75"
set "MAX_DELAY=60"
set /a delay=1

:loop
"%BIN%" %*
set "code=!ERRORLEVEL!"

if "!code!"=="0" (
	echo [artex] exited normally
	exit /b 0
)

if "!code!"=="%RESTART_CODE%" (
	rem Update/rollback is ready: after rerunning, artex completes the swap at startup.
	echo [artex] restart requested (applying new version)…
	set /a delay=1
	goto loop
)

echo [artex] exited abnormally ^(code=!code!^), restarting in !delay!s 1>&2
rem timeout fails in a redirected console; use ping as a fallback (N seconds of delay needs N+1 pings).
set /a pings=!delay!+1
ping -n !pings! 127.0.0.1 >nul 2>&1
set /a delay=!delay!*2
if !delay! gtr %MAX_DELAY% set /a delay=%MAX_DELAY%
goto loop