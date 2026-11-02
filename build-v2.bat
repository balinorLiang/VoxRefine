@echo off
setlocal
cd /d "%~dp0"
if not exist bin mkdir bin
go build -trimpath -ldflags "-s -w" -o bin\polyphone-server.exe .
echo Built bin\polyphone-server.exe