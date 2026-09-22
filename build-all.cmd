@echo off
setlocal EnableExtensions
cd /d "%~dp0"

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
if not errorlevel 1 (
  for /f "delims=" %%B in ('where bun 2^>nul') do if not defined BUN set "BUN=%%B"
)
if not defined BUN if exist "%USERPROFILE%\.bun\bin\bun.exe" set "BUN=%USERPROFILE%\.bun\bin\bun.exe"
if not defined BUN if exist "%LOCALAPPDATA%\Programs\bun\bun.exe" set "BUN=%LOCALAPPDATA%\Programs\bun\bun.exe"
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
