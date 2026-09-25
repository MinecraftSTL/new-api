@echo off
setlocal EnableExtensions
cd /d "%~dp0"

rem Use the Windows system HTTP proxy when enabled.
call :load_system_proxy
if not defined HTTP_PROXY if defined HTTPS_PROXY set "HTTP_PROXY=%HTTPS_PROXY%"
if not defined HTTPS_PROXY if defined HTTP_PROXY set "HTTPS_PROXY=%HTTP_PROXY%"

set "OUT_DIR=build"
set "TARGETS=linux/amd64 linux/arm64 windows/amd64 windows/arm64 darwin/amd64 darwin/arm64"

if not exist "%OUT_DIR%" mkdir "%OUT_DIR%"

where go >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Go was not found in PATH.
  exit /b 1
)

set "BUN="
where bun >nul 2>nul
if not errorlevel 1 for /f "delims=" %%B in ('where bun 2^>nul') do if not defined BUN set "BUN=%%B"
if not defined BUN if exist "%USERPROFILE%\.bun\bin\bun.exe" set "BUN=%USERPROFILE%\.bun\bin\bun.exe"
if not defined BUN if exist "%LOCALAPPDATA%\Programs\bun\bun.exe" set "BUN=%LOCALAPPDATA%\Programs\bun\bun.exe"
if not defined BUN if exist "%LOCALAPPDATA%\Microsoft\WinGet\Links\bun.exe" set "BUN=%LOCALAPPDATA%\Microsoft\WinGet\Links\bun.exe"
if not defined BUN if exist "%APPDATA%\npm\bun.cmd" set "BUN=%APPDATA%\npm\bun.cmd"
if not defined BUN if exist "%LOCALAPPDATA%\pnpm\bun.exe" set "BUN=%LOCALAPPDATA%\pnpm\bun.exe"
for /d %%D in ("%LOCALAPPDATA%\npm-cache\_npx\*") do if not defined BUN if exist "%%~fD\node_modules\bun\bin\bun.exe" set "BUN=%%~fD\node_modules\bun\bin\bun.exe"
for /d %%D in ("%LOCALAPPDATA%\pnpm\store\v11\projects\*") do if not defined BUN if exist "%%~fD\node_modules\bun\bin\bun.exe" set "BUN=%%~fD\node_modules\bun\bin\bun.exe"
for /d %%D in ("%LOCALAPPDATA%\pnpm-cache\dlx\*") do if not defined BUN if exist "%%~fD\pkg\node_modules\bun\bin\bun.exe" set "BUN=%%~fD\pkg\node_modules\bun\bin\bun.exe"
for /d %%D in ("%LOCALAPPDATA%\pnpm-cache\dlx\*") do if not defined BUN if exist "%%~fD\node_modules\bun\bin\bun.exe" set "BUN=%%~fD\node_modules\bun\bin\bun.exe"
if not defined BUN (
  echo [ERROR] Bun was not found in PATH or common install locations.
  echo         Install Bun 1.4.x or add bun.exe to PATH.
  exit /b 1
)

set "APP_VERSION="
if exist VERSION set /p "APP_VERSION="<VERSION
if not defined APP_VERSION for /f "delims=" %%V in ('git describe --tags --always --dirty 2^>nul') do if not defined APP_VERSION set "APP_VERSION=%%V"
if not defined APP_VERSION set "APP_VERSION=dev"

echo [1/2] Building web frontend...
pushd web
call "%BUN%" install --frozen-lockfile
if errorlevel 1 (
  popd
  goto :fail
)
set "DISABLE_ESLINT_PLUGIN=true"
set "VITE_REACT_APP_VERSION=%APP_VERSION%"
call "%BUN%" run build
if errorlevel 1 (
  popd
  goto :fail
)
popd

echo [2/2] Building Go binaries for %TARGETS%
for %%T in (%TARGETS%) do (
  for /f "tokens=1,2 delims=/" %%A in ("%%T") do (
    call :build_target "%%A" "%%B"
    if errorlevel 1 goto :fail
  )
)

echo.
echo Build complete. Output directory:
echo   %CD%\%OUT_DIR%
echo.
echo Files:
for %%F in ("%OUT_DIR%\new-api-*") do echo   %%~nxF
exit /b 0

:load_system_proxy
set "PROXY_ENABLE="
set "PROXY_SERVER="
for /f "tokens=2,*" %%A in ('reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable 2^>nul ^| findstr /I "ProxyEnable"') do set "PROXY_ENABLE=%%B"
if /I not "%PROXY_ENABLE%"=="0x1" exit /b 0
for /f "tokens=2,*" %%A in ('reg query "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyServer 2^>nul ^| findstr /I "ProxyServer"') do set "PROXY_SERVER=%%B"
if not defined PROXY_SERVER exit /b 0
set "PROXY_HAS_SCHEME="
for /f "tokens=1,* delims==" %%A in ("%PROXY_SERVER%") do if not "%%B"=="" set "PROXY_HAS_SCHEME=1"
if not defined PROXY_HAS_SCHEME (
  set "HTTP_PROXY=http://%PROXY_SERVER%"
  exit /b 0
)
for %%P in (%PROXY_SERVER:;= %) do (
  for /f "tokens=1,* delims==" %%A in ("%%P") do (
    if /I "%%A"=="http" set "HTTP_PROXY=http://%%B"
    if /I "%%A"=="https" set "HTTPS_PROXY=http://%%B"
    if /I "%%A"=="socks" set "HTTP_PROXY=socks5://%%B"
    if /I "%%A"=="socks" set "HTTPS_PROXY=socks5://%%B"
    if /I "%%A"=="socks5" set "HTTP_PROXY=socks5://%%B"
    if /I "%%A"=="socks5" set "HTTPS_PROXY=socks5://%%B"
  )
)
exit /b 0

:build_target
set "TARGET_OS=%~1"
set "TARGET_ARCH=%~2"
set "OUT_FILE=%OUT_DIR%\new-api-%TARGET_OS%-%TARGET_ARCH%"
if /I "%TARGET_OS%"=="windows" set "OUT_FILE=%OUT_FILE%.exe"
set "OUT_TMP=%OUT_FILE%.tmp"

echo   Building %TARGET_OS%/%TARGET_ARCH% -^> %OUT_FILE%
set "CGO_ENABLED=0"
set "GOOS=%TARGET_OS%"
set "GOARCH=%TARGET_ARCH%"
set "GOWORK=off"
del /q "%OUT_TMP%" 2>nul
go build -trimpath -ldflags "-s -w -X github.com/QuantumNous/new-api/common.Version=%APP_VERSION%" -o "%OUT_TMP%" .
if errorlevel 1 (
  del /q "%OUT_TMP%" 2>nul
  exit /b 1
)
move /y "%OUT_TMP%" "%OUT_FILE%" >nul
if errorlevel 1 (
  del /q "%OUT_TMP%" 2>nul
  exit /b 1
)
exit /b 0

:fail
echo.
echo [ERROR] Build failed.
exit /b 1
