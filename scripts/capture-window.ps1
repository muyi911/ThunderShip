param(
    [Parameter(Mandatory)][int]$ProcessId,
    [Parameter(Mandatory)][string]$OutputPath
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class WindowCapture {
    [StructLayout(LayoutKind.Sequential)] public struct Rect { public int Left, Top, Right, Bottom; }
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hwnd, out Rect rect);
    [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr hwnd, IntPtr hdc, uint flags);
}
'@
$appProcess = Get-Process -Id $ProcessId
if ($appProcess.MainWindowTitle -ne '霆舟 ThunderShip') { throw 'Target must be a ThunderShip window.' }
$rect = New-Object WindowCapture+Rect
if (-not [WindowCapture]::GetWindowRect($appProcess.MainWindowHandle, [ref]$rect)) { throw 'Cannot read window bounds.' }
$bitmap = New-Object System.Drawing.Bitmap(($rect.Right - $rect.Left), ($rect.Bottom - $rect.Top))
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$hdc = $graphics.GetHdc()
try {
    if (-not [WindowCapture]::PrintWindow($appProcess.MainWindowHandle, $hdc, 2)) { throw 'Window capture failed.' }
} finally { $graphics.ReleaseHdc($hdc); $graphics.Dispose() }
try { $bitmap.Save([IO.Path]::GetFullPath($OutputPath), [System.Drawing.Imaging.ImageFormat]::Png) }
finally { $bitmap.Dispose() }
