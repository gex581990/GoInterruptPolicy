# Version, User, Buildtime https://www.digitalocean.com/community/tutorials/using-ldflags-to-set-version-information-for-go-applications
Param(
    [string]$version = "0.0.0.0",

    [Parameter(ValueFromRemainingArguments)]
    [string[]]$rest
)

$env:GOOS = "windows"
$env:GOARCH = "amd64"
$filename = "GoInterruptPolicy"

if ($version -eq "") {
    $major = 0
    $minor = 0
    $patch = 0
    $build = [int]([datetime]::now).tostring("yyyyMMdd")
    $versionNumber = "0.0.0.$build"
} else {
    $versionArray = $version -split "\."
    $major = [int]$versionArray[0]
    $minor = [int]$versionArray[1]
    $patch = [int]$versionArray[2]
    $build = [int]$versionArray[3]
    $versionNumber = "$major.$minor.$patch.$build"
}

if ($rest -ne "") {
    $versionNumber = $versionNumber + "_" + $rest -Join '_'
}

# https://github.com/josephspurrier/goversioninfo#command-line-flags
goversioninfo `
    -file-version="$versionNumber" `
    -ver-major="$major" `
    -ver-minor="$minor" `
    -ver-patch="$patch" `
    -ver-build="$build" `
    -product-version="$versionNumber" `
    -product-ver-major="$major" `
    -product-ver-minor="$minor" `
    -product-ver-patch="$patch" `
    -product-ver-build="$build"

while ($true) {
    Clear-Host

    go-critic check -enableAll -disable "#experimental,#opinionated,#commentedOutCode" ./... 2>$null

    if (Test-Path "$filename.exe") {
        $beforeSize = (Get-Item "$filename.exe").Length
    } else {
        $beforeSize = 0
    }

    # Build (Go)
    #   -w disable DWARF generation
    #   -s disable symbol table
    #   -H set header type
    #   -X add string value definition of the form importpath.name=value
    go build -o "${filename}_debug.exe" -tags debug -buildvcs=false
    go build -o $filename`.exe -trimpath -ldflags "-w -s -H windowsgui -X main.Version=$versionNumber"

    $size = (Get-Item "$filename.exe").Length
    $diffSize = $size - $beforeSize
    $sizeKb = [math]::Ceiling($size / 1024)

    if ($diffSize -eq 0) {
        Write-Host "$sizeKb kb"
    } else {
        if ($diffSize -gt 0) {
            Write-Host "$sizeKb kb [+$diffSize b]"
        } else {
            Write-Host "$sizeKb kb [$diffSize b]"
        }
    }

    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
}
