$link = "https://github.com/Slipcords/Slipped/releases/latest/download/SlippedCli.exe"

$outfile = "$env:TEMP\SlippedCli.exe"

Write-Output "Downloading installer to $outfile"

Invoke-WebRequest -Uri "$link" -OutFile "$outfile"

Write-Output ""

Start-Process -Wait -NoNewWindow -FilePath "$outfile"

# Cleanup
Remove-Item -Force "$outfile"
