@echo off
chcp 65001 >nul

echo [1/3] 开始构建前端 Vue3 项目...
cd web
call npm run build
if %errorlevel% neq 0 (
    echo [错误] 前端构建失败，请检查前端依赖或语法。
    cd ..
    exit /b %errorlevel%
)
cd ..

echo.
echo [2/3] 设置 Linux amd64 交叉编译环境变量...
setlocal
set GOOS=linux
set GOARCH=amd64

echo.
echo [3/3] 开始编译 Go 后端二进制 (注入 vue 静态标签)...
go build -tags vue -ldflags="-s -w" -o fluxor
if %errorlevel% neq 0 (
    echo [错误] Go 后端二进制编译失败。
    endlocal
    exit /b %errorlevel%
)
endlocal

echo.
echo [成功] Linux amd64 二进制文件 "fluxor" 编译完成。
pause
