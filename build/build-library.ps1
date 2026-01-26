param(
    [string]$ExcelPath = "My Library.xlsx",
    [string]$BooksJsonPath = "data\books.json",
    [string]$BooksJsPath = "data\books.js",
    [string]$CoversDirectory = "data\covers",
    [string]$ManualCoversDirectory = "data\manual-covers"
)

# --- Path Resolution ---
$ScriptRoot = $PSScriptRoot
# When running in /build, ProjectRoot is one level up
$ProjectRoot = Split-Path -Parent $ScriptRoot

# Resolve Excel Path
$ExcelFullPath = Join-Path $ProjectRoot $ExcelPath

$BooksJsonPath = Join-Path $ProjectRoot $BooksJsonPath
$BooksJsPath = Join-Path $ProjectRoot $BooksJsPath
$CoversDirectory = Join-Path $ProjectRoot $CoversDirectory
$ManualCoversDirectory = Join-Path $ProjectRoot $ManualCoversDirectory

# --- Dependencies ---
try { Add-Type -AssemblyName System.Drawing -ErrorAction Stop }
catch { Write-Error "System.Drawing is required."; exit 1 }

# --- Helper Functions ---

function Ensure-Directory {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        New-Item -ItemType Directory -Force -Path $Path | Out-Null
    }
}

function New-Slug {
    param([string]$Seed)
    if ([string]::IsNullOrWhiteSpace($Seed)) { return [Guid]::NewGuid().ToString('N').Substring(0, 12) }
    $slug = [System.Text.RegularExpressions.Regex]::Replace($Seed.ToLowerInvariant(), "[^a-z0-9]+", "-").Trim('-')
    if ($slug) { return $slug } else { return [Guid]::NewGuid().ToString('N').Substring(0, 12) }
}

function Get-GoodreadsCover {
    param([string]$Isbn)
    
    $url = "https://www.goodreads.com/book/isbn/$Isbn"
    
    # Throttling
    $delay = 0
    # Write-Host "   [Wait ${delay}s]" -NoNewline -ForegroundColor DarkGray
    # Start-Sleep -Seconds $delay

    try {
        $userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
        $r = Invoke-WebRequest -Uri $url -UserAgent $userAgent -UseBasicParsing -ErrorAction Stop
        
        if ($r.StatusCode -ne 200) { return $null }
        $content = $r.Content
        
        # Bot Check
        if ($content -match "captcha" -or $content -match "robot check") { return "BLOCKED" }

        $img = $null
        # Strategies
        if ($content -match '<meta\s+property="og:image"\s+content="([^"]+)"') { $img = $matches[1] }
        elseif ($content -match '<meta\s+content="([^"]+)"\s+property="og:image"') { $img = $matches[1] }
        
        if (-not $img -and $content -match 'class="bookCover"\s+[^>]*src="([^"]+)"') { $img = $matches[1] }
        if (-not $img -and $content -match 'id="coverImage"[^>]+src="([^"]+)"') { $img = $matches[1] }

        if ($img -match "nophoto") { return $null }
        
        if ($img) {
            return ($img -replace '\._S[XY]\d+_', '')
        }
    } catch {
        $status = if ($_.Exception.Response) { $_.Exception.Response.StatusCode } else { "Err" }
        if ($status -eq 429 -or $status -eq 503) { return "BLOCKED" }
    }
    return $null
}

function Get-SpinePalette {
    param([string]$ImagePath)
    if (-not (Test-Path -LiteralPath $ImagePath)) { return $null }

    try {
        $original = [System.Drawing.Image]::FromFile($ImagePath)
        $targetWidth = 60
        $targetHeight = [Math]::Max([int]($original.Height * ($targetWidth / [double]$original.Width)), 40)
        $scaled = New-Object System.Drawing.Bitmap $targetWidth, $targetHeight
        $graphics = [System.Drawing.Graphics]::FromImage($scaled)
        $graphics.DrawImage($original, 0, 0, $targetWidth, $targetHeight)
        $graphics.Dispose()
        $original.Dispose()

        $r = 0; $g = 0; $b = 0; $count = 0
        for ($x = 0; $x -lt $scaled.Width; $x++) {
            for ($y = 0; $y -lt $scaled.Height; $y++) {
                $pixel = $scaled.GetPixel($x, $y)
                if ($pixel.A -le 15) { continue }
                $r += $pixel.R; $g += $pixel.G; $b += $pixel.B; $count++
            }
        }
        $scaled.Dispose()
        
        if ($count -eq 0) { return $null }
        $avgR = [int]($r / $count); $avgG = [int]($g / $count); $avgB = [int]($b / $count)
        $hex = "#{0:X2}{1:X2}{2:X2}" -f $avgR, $avgG, $avgB
        $luminance = (0.2126 * $avgR + 0.7152 * $avgG + 0.0722 * $avgB) / 255
        return @{ background = $hex; text = if ($luminance -gt 0.55) { "#1c1c22" } else { "#fdfdfd" } }
    } catch { return $null }
}

# --- Main Logic ---

function Show-SubMenu {
    param([string]$SourceName)
    Write-Host "`nHow do you want to run $SourceName?" -ForegroundColor Cyan
    Write-Host "1. Rebuild from ground up (Redownload All)"
    Write-Host "2. Update (Only fetch missing)"
    return Read-Host "Select option"
}

Clear-Host
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "      Bookshelf Library Builder" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "1. Open Library (Fast, Low Accuracy)"
Write-Host "2. Goodreads    (Slow, High Accuracy)"
Write-Host "3. Offline Mode (Rebuild JSON only)"
Write-Host "4. Apply Manual Covers"
Write-Host "Q. Quit"
Write-Host "----------------------------------------"

$choice = Read-Host "Choose an option"
if ($choice -in 'q', 'Q') { exit }

$fetchMode = "None"     # None, OL, GR
$forceRebuild = $false  # If true, file existence check is skipped
$onlyManuals = $false

switch ($choice) {
    '1' { 
        $fetchMode = "OL"
        $sub = Show-SubMenu "Open Library"
        if ($sub -eq '1') { $forceRebuild = $true }
    }
    '2' { 
        $fetchMode = "GR"
        $sub = Show-SubMenu "Goodreads"
        if ($sub -eq '1') { $forceRebuild = $true }
    }
    '3' { 
        # Offline - Defaults apply
    }
    '4' {
        $onlyManuals = $true
    }
    Default { Write-Warning "Invalid option. Exiting."; exit }
}

# --- Read Data ---
# --- Read Data ---
$books = @()

if (Test-Path $ExcelFullPath) {
    Write-Host "`nReading Excel Library..." -ForegroundColor Cyan
    $excel = New-Object -ComObject Excel.Application
    $excel.Visible = $false
    
    try {
        $workbook = $excel.Workbooks.Open($ExcelFullPath)
        $sheet = $workbook.Worksheets.Item(1)
        $usedRange = $sheet.UsedRange
        $rows = $usedRange.Rows.Count
        $cols = $usedRange.Columns.Count

        $headers = @()
        for ($c = 1; $c -le $cols; $c++) {
            $val = $usedRange.Cells.Item(1, $c).Text.Trim()
            $headers += if ($val) { $val } else { "Col$c" }
        }

        for ($r = 2; $r -le $rows; $r++) {
            $rowMap = @{}
            $hasData = $false
            for ($c = 1; $c -le $cols; $c++) {
                $val = $usedRange.Cells.Item($r, $c).Text
                if (-not [string]::IsNullOrWhiteSpace($val)) { $hasData = $true }
                $rowMap[$headers[$c - 1]] = $val
            }
            if (-not $hasData) { continue }

            $title = $rowMap['Title']; $author = $rowMap['Author']; $isbn = $rowMap['ISBN']
            if (-not $title) { continue }

            $slugSource = if ($isbn) { $isbn } else { "$title-$author" }
            $publishedYear = if ($rowMap['Published'] -match '\d{4}') { [int]$matches[0] } else { $null }

            $bk = [pscustomobject]@{
                id = New-Slug $slugSource
                title = $title
                author = $author
                isbn = $isbn
                translator = $rowMap['Translator']
                publisher = $rowMap['Publisher']
                binding = $rowMap['Binding']
                published = $publishedYear
                cover = $null; spineColor = $null; spineTextColor = $null
            }
            $books += $bk
        }
    } finally {
        $workbook.Close($false)
        $excel.Quit()
        [System.Runtime.Interopservices.Marshal]::ReleaseComObject($excel) | Out-Null
    }
} elseif (Test-Path $BooksJsonPath) {
    Write-Host "`nReading existing JSON (Excel not found)..." -ForegroundColor Yellow
    $jsonContent = Get-Content -LiteralPath $BooksJsonPath -Raw -Encoding UTF8
    $books = $jsonContent | ConvertFrom-Json
    
    # Ensure objects have required properties (Json deserialization makes them pscustomobjects already)
    # But properties might be missing if they weren't in the source JSON.
    foreach ($b in $books) {
        if (-not $b.psobject.Properties.Match('cover').Count) {
             $b | Add-Member -MemberType NoteProperty -Name 'cover' -Value $null
        }
        if (-not $b.psobject.Properties.Match('spineColor').Count) {
             $b | Add-Member -MemberType NoteProperty -Name 'spineColor' -Value $null
        }
        if (-not $b.psobject.Properties.Match('spineTextColor').Count) {
             $b | Add-Member -MemberType NoteProperty -Name 'spineTextColor' -Value $null
        }
    }
} else {
    Write-Error "Could not find Excel file at: $ExcelFullPath OR JSON at $BooksJsonPath"
    exit
}

Ensure-Directory $CoversDirectory
Ensure-Directory $ManualCoversDirectory

# --- Processing Loop ---
Write-Host "Processing $($books.Count) books..."
$stats = @{ manuals = 0; downloads = 0; colored = 0; skipped = 0 }

foreach ($book in $books) {
    $isbnClean = if ($book.isbn) { ($book.isbn -replace "[^0-9Xx]", "").ToUpperInvariant() } else { "" }
    $fileName = if ($isbnClean) { "$isbnClean.jpg" } else { "$($book.id).jpg" }
    $destPath = Join-Path $CoversDirectory $fileName
    
    # 1. Manual Covers (Highest Priority)
    # Check Manual Dir
    $manualSrc = $null
    $candidates = @($isbnClean, $book.id)
    foreach ($c in $candidates) {
        if (-not $c) { continue }
        foreach ($ext in @('.jpg','.png','.jpeg','.webp')) {
            $p = Join-Path $ManualCoversDirectory "$c$ext"
            if (Test-Path $p) { $manualSrc = $p; break }
        }
        if ($manualSrc) { break }
    }

    if ($manualSrc) {
        # If Option 4 (Manuals only) or normal rebuild, or missing target
        if ($onlyManuals -or $forceRebuild -or (-not (Test-Path $destPath))) {
            Write-Host "  [$($book.title)] Applying Manual Cover..." -NoNewline
            try {
                $img = [System.Drawing.Image]::FromFile($manualSrc)
                $img.Save($destPath, [System.Drawing.Imaging.ImageFormat]::Jpeg)
                $img.Dispose()
                Write-Host " Done." -ForegroundColor Green
                $stats.manuals++
            } catch { Write-Host " Error." -ForegroundColor Red }
        }
    } 
    # 2. Online Fetching
    elseif (-not $onlyManuals -and $fetchMode -ne "None" -and $isbnClean) {
        
        # Skip if exists and not force rebuild
        if ((Test-Path $destPath) -and -not $forceRebuild) {
            # Skip
        } else {
            $dlUrl = $null
            
            # Open Library
            if ($fetchMode -eq "OL") {
                Write-Host "  [$($book.title)] Checking OpenLibrary..." -NoNewline
                $dlUrl = "https://covers.openlibrary.org/b/isbn/$isbnClean-L.jpg?default=false"
                # OL logic is simple, assume url is valid for try
            }
            # Goodreads
            elseif ($fetchMode -eq "GR") {
                Write-Host "  [$($book.title)] Fetching Goodreads..." -NoNewline
                $dlUrl = Get-GoodreadsCover -Isbn $isbnClean
                if ($dlUrl -eq "BLOCKED") {
                    Write-Host " !!! BLOCKED - Stopping Network !!!" -ForegroundColor Red
                    $fetchMode = "None" # Kill switch
                    $dlUrl = $null
                }
            }

            if ($dlUrl) {
                try {
                    Invoke-WebRequest -Uri $dlUrl -OutFile $destPath -UseBasicParsing -ErrorAction Stop | Out-Null
                    if ((Get-Item $destPath).Length -lt 1000) {
                         Remove-Item $destPath -Force
                         Write-Host " Invalid/Empty." -ForegroundColor Red
                    } else {
                        Write-Host " Downloaded." -ForegroundColor Green
                        $stats.downloads++
                    }
                } catch { 
                     Write-Host " Failed/Not Found." -ForegroundColor DarkGray
                }
            } else {
                if ($fetchMode -ne "None") { Write-Host " No Cover Found." -ForegroundColor DarkGray }
            }
        }
    }

    # 3. Finalize Data (Color Extraction)
    if (Test-Path $destPath) {
        $book.cover = "data/covers/$fileName"
        $palette = Get-SpinePalette -ImagePath $destPath
        if ($palette) {
            $book.spineColor = $palette.background
            $book.spineTextColor = $palette.text
            $stats.colored++
        }
    } else {
        $stats.skipped++
    }
}

# --- Save ---
$json = $books | ConvertTo-Json -Depth 4
$json | Set-Content -LiteralPath $BooksJsonPath -Encoding UTF8
"window.booksData = $json;" | Set-Content -LiteralPath $BooksJsPath -Encoding UTF8

Write-Host "`nDONE! | Manuals: $($stats.manuals) | Downloads: $($stats.downloads) | Colored: $($stats.colored)" -ForegroundColor Green
Read-Host "Press Enter to exit..."

