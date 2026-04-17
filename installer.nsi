; DioupeCam Desktop Installer
; Empacota: DioupeCamDesktop.exe + ffmpeg.exe + DioupeCamFilter DLLs

Unicode True

!define APP_NAME "DioupeCam Desktop"
!define APP_VERSION "1.0.0"
!define APP_EXE "DioupeCamDesktop.exe"
!define REG_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\DioupeCam"

Name "${APP_NAME}"
OutFile "build\DioupeCamDesktop-Setup.exe"
InstallDir "$PROGRAMFILES64\DioupeCam"
InstallDirRegKey HKLM "${REG_KEY}" "InstallLocation"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

;--------------------------------
; Pages
Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

;--------------------------------
Section "Principal" SecMain
  SetOutPath "$INSTDIR"

  ; App principal
  File "build\bin\DioupeCamDesktop.exe"

  ; FFmpeg (buscado pelo app no mesmo diretório)
  File "deps\ffmpeg.exe"

  ; Filtro DirectShow — câmera virtual "DioupeCam"
  File "deps\DioupeCamFilter32.dll"
  File "deps\DioupeCamFilter64.dll"

  ; Registrar filtro
  ExecWait '"$SYSDIR\regsvr32.exe" /s "$INSTDIR\DioupeCamFilter32.dll"'
  ExecWait '"$SYSDIR\regsvr32.exe" /s "$INSTDIR\DioupeCamFilter64.dll"'

  ; Instalar WebView2 Runtime se não estiver presente (necessário para a UI do app)
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  StrCmp $0 "" 0 webview2_ok
    DetailPrint "Instalando WebView2 Runtime..."
    ExecWait 'powershell -Command "Invoke-WebRequest -Uri https://go.microsoft.com/fwlink/p/?LinkId=2124703 -OutFile $$env:TEMP\wv2setup.exe; Start-Process $$env:TEMP\wv2setup.exe -ArgumentList /silent,/install -Wait"'
  webview2_ok:

  ; Atalhos
  CreateDirectory "$SMPROGRAMS\DioupeCam"
  CreateShortcut "$SMPROGRAMS\DioupeCam\DioupeCam Desktop.lnk" "$INSTDIR\${APP_EXE}"
  CreateShortcut "$SMPROGRAMS\DioupeCam\Desinstalar.lnk" "$INSTDIR\Uninstall.exe"
  CreateShortcut "$DESKTOP\DioupeCam Desktop.lnk" "$INSTDIR\${APP_EXE}"

  ; Registrar desinstalador no Painel de Controle
  WriteRegStr HKLM "${REG_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKLM "${REG_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKLM "${REG_KEY}" "Publisher" "DioupeCam"
  WriteRegStr HKLM "${REG_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${REG_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKLM "${REG_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${REG_KEY}" "NoRepair" 1

  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

;--------------------------------
Section "Uninstall"
  ; Desregistrar filtro
  ExecWait '"$SYSDIR\regsvr32.exe" /s /u "$INSTDIR\DioupeCamFilter32.dll"'
  ExecWait '"$SYSDIR\regsvr32.exe" /s /u "$INSTDIR\DioupeCamFilter64.dll"'

  ; Remover atalhos
  Delete "$SMPROGRAMS\DioupeCam\DioupeCam Desktop.lnk"
  Delete "$SMPROGRAMS\DioupeCam\Desinstalar.lnk"
  RMDir "$SMPROGRAMS\DioupeCam"
  Delete "$DESKTOP\DioupeCam Desktop.lnk"

  ; Remover arquivos
  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\ffmpeg.exe"
  Delete "$INSTDIR\DioupeCamFilter32.dll"
  Delete "$INSTDIR\DioupeCamFilter64.dll"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  ; Remover entrada no Painel de Controle
  DeleteRegKey HKLM "${REG_KEY}"
SectionEnd
