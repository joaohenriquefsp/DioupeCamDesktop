@echo off
cd /d "C:\Users\JOOHEN~1\Pessoal\DioupeCamDesktop"
echo [1/2] Building DioupeCamDesktop.exe...
wails build -o DioupeCamDesktop.exe
if %ERRORLEVEL% NEQ 0 ( echo WAILS BUILD FAILED & goto end )
echo [2/2] Building installer...
"C:\Program Files (x86)\NSIS\makensis.exe" installer.nsi
if %ERRORLEVEL% NEQ 0 ( echo NSIS FAILED & goto end )
echo ALL DONE
:end
echo EXIT: %ERRORLEVEL%
