@echo off
cd /d "C:\Users\JOOHEN~1\Pessoal\DioupeCamDesktop"
"C:\Program Files (x86)\NSIS\makensis.exe" installer.nsi
echo EXIT: %ERRORLEVEL%
