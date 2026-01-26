#!/usr/bin/env python3
import os
import sys
import json
import shutil
import re
import zipfile
import urllib.request
import subprocess
import xml.etree.ElementTree as ET
from datetime import datetime

# --- Configuration ---
EXCEL_PATH = "My Library.xlsx"
BOOKS_JSON_PATH = os.path.join("data", "books.json")
BOOKS_JS_PATH = os.path.join("data", "books.js")
COVERS_DIR = os.path.join("data", "covers")
MANUAL_COVERS_DIR = os.path.join("data", "manual-covers")
OPEN_LIBRARY_URL = "https://covers.openlibrary.org/b/isbn/{isbn}-L.jpg?default=false"

# --- Helper Classes & Functions ---

class SimpleXlsxReader:
    """
    A minimal, dependency-free XLSX reader using standard Python libraries.
    Reads the first sheet and treats the first row as headers.
    """
    def __init__(self, filepath):
        self.filepath = filepath
        self.shared_strings = []
        self.ns = {'a': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}

    def read(self):
        if not os.path.exists(self.filepath):
            return []

        try:
            with zipfile.ZipFile(self.filepath, 'r') as z:
                # 1. Parse Shared Strings
                if 'xl/sharedStrings.xml' in z.namelist():
                    with z.open('xl/sharedStrings.xml') as f:
                        tree = ET.parse(f)
                        root = tree.getroot()
                        # Each <si> is a shared string item, usually containing <t>
                        for si in root.findall('{http://schemas.openxmlformats.org/spreadsheetml/2006/main}si'):
                            t = si.find('{http://schemas.openxmlformats.org/spreadsheetml/2006/main}t')
                            if t is not None and t.text:
                                self.shared_strings.append(t.text)
                            else:
                                # Sometimes text is inside formatting runs <r><t>
                                text = ""
                                for t_node in si.findall('.//{http://schemas.openxmlformats.org/spreadsheetml/2006/main}t'):
                                    if t_node.text:
                                        text += t_node.text
                                self.shared_strings.append(text)

                # 2. Parse Sheet 1
                if 'xl/worksheets/sheet1.xml' not in z.namelist():
                    return []
                
                rows_data = []
                with z.open('xl/worksheets/sheet1.xml') as f:
                    tree = ET.parse(f)
                    root = tree.getroot()
                    sheet_data = root.find('{http://schemas.openxmlformats.org/spreadsheetml/2006/main}sheetData')
                    
                    headers = {}
                    
                    for row in sheet_data.findall('{http://schemas.openxmlformats.org/spreadsheetml/2006/main}row'):
                        row_idx = int(row.get('r'))
                        cells = row.findall('{http://schemas.openxmlformats.org/spreadsheetml/2006/main}c')
                        
                        row_values = {}
                        
                        for cell in cells:
                            coord = cell.get('r') # e.g. A1, B2
                            col_letter = "".join(filter(str.isalpha, coord))
                            val_type = cell.get('t')
                            val_node = cell.find('{http://schemas.openxmlformats.org/spreadsheetml/2006/main}v')
                            
                            val = ""
                            if val_node is not None:
                                raw_val = val_node.text
                                if val_type == 's': # Shared String
                                    idx = int(raw_val)
                                    if idx < len(self.shared_strings):
                                        val = self.shared_strings[idx]
                                elif val_type == 'str': # Inline String
                                    val = raw_val
                                else: # Number or other
                                    val = raw_val
                            
                            row_values[col_letter] = val.strip() if val else ""

                        # Assume Row 1 is headers
                        if row_idx == 1:
                            col_index = 0
                            for cell in cells:
                                coord = cell.get('r')
                                col = "".join(filter(str.isalpha, coord))
                                # Map 'A' -> 'Title', etc.
                                if col in row_values:
                                    headers[col] = row_values[col]
                            continue

                        # Data Rows
                        item = {}
                        is_empty = True
                        for col, header_name in headers.items():
                            val = row_values.get(col, "")
                            if val:
                                is_empty = False
                            item[header_name] = val
                            
                        if not is_empty and 'Title' in item and item['Title']:
                            rows_data.append(item)
                            
            return rows_data

        except Exception as e:
            print(f"Error reading Excel file: {e}")
            return []

def ensure_directory(path):
    if not os.path.exists(path):
        os.makedirs(path)

def normalize_field(val):
    if not val:
        return None
    val = str(val).strip()
    return val if val else None

def new_slug(seed):
    import uuid
    if not seed:
        return str(uuid.uuid4())[:12]
    
    slug = re.sub(r'[^a-z0-9]+', '-', seed.lower()).strip('-')
    if not slug:
        return str(uuid.uuid4())[:12]
    return slug

def parse_year(date_str):
    if not date_str:
        return None
    match = re.search(r'\d{4}', str(date_str))
    if match:
        return int(match.group(0))
    return None

def get_manual_cover_file(book, isbn_clean, manual_dir):
    if not os.path.exists(manual_dir):
        return None
    
    candidates = []
    if isbn_clean: candidates.append(isbn_clean)
    if book.get('id'): candidates.append(book['id'])
    
    extensions = ['.jpg', '.jpeg', '.png', '.webp', '.bmp']
    
    for name in candidates:
        for ext in extensions:
            path = os.path.join(manual_dir, name + ext)
            if os.path.exists(path):
                return path
    return None

def get_goodreads_cover(isbn):
    import time
    import random
    
    url = f"https://www.goodreads.com/book/isbn/{isbn}"
    
    # Throttling
    delay = random.uniform(1, 3)
    print(f"   [Wait {delay:.1f}s]", end="", flush=True)
    time.sleep(delay)

    try:
        req = urllib.request.Request(url, headers={
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'
        })
        with urllib.request.urlopen(req) as r:
            if r.status != 200: return None
            content = r.read().decode('utf-8', errors='ignore')

        # Bot Check
        if "captcha" in content or "robot check" in content:
            return "BLOCKED"

        img = None
        # Strategies
        m = re.search(r'<meta\s+property="og:image"\s+content="([^"]+)"', content)
        if m: img = m.group(1)
        
        if not img:
            m = re.search(r'<meta\s+content="([^"]+)"\s+property="og:image"', content)
            if m: img = m.group(1)

        if not img:
            m = re.search(r'class="bookCover"\s+[^>]*src="([^"]+)"', content)
            if m: img = m.group(1)

        if not img:
            m = re.search(r'id="coverImage"[^>]+src="([^"]+)"', content)
            if m: img = m.group(1)

        if img and "nophoto" in img: return None

        if img:
            # Remove resizing suffix like ._SX98_. or ._SY475_
            return re.sub(r'\._S[XY]\d+_', '', img)
            
    except Exception as e:
        # Check for 429/503 via exception structure if using urllib.error
        # Minimal handling for now
        if hasattr(e, 'code') and (e.code == 429 or e.code == 503):
             return "BLOCKED"
        pass
        
    return None

def extract_spine_colors_imagemagick(image_path):
    """
    Extracts average color and determines text color using ImageMagick 'convert'.
    Returns: {'background': '#RRGGBB', 'text': '#RRGGBB'} or None
    """
    if not shutil.which('convert'):
        return None

    try:
        # Resize to 1x1 and output hex text
        cmd = ['convert', image_path, '-resize', '1x1', '-format', '%[hex:u.p{0,0}]', 'info:']
        result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        
        hex_color = result.stdout.strip()
        if not hex_color or len(hex_color) < 6:
            return None
            
        # Ensure # prefix (ImageMagick sometimes omits it with this format string)
        bg_color = f"#{hex_color[:6]}"
        
        # Calculate Luminance
        r = int(hex_color[0:2], 16)
        g = int(hex_color[2:4], 16)
        b = int(hex_color[4:6], 16)
        
        luminance = (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255
        text_color = "#1c1c22" if luminance > 0.55 else "#fdfdfd"
        
        return {'background': bg_color, 'text': text_color}
        
    except Exception:
        return None

# --- Main Logic ---

def main():
    # MENU
    print("===========================================")
    print("   Bookshelf Library Builder (Linux/Python)")
    print("===========================================")
    print("2. Goodreads    (Slow, High Accuracy)")
    print("3. Offline Mode (Rebuild JSON only)")
    print("4. Apply Manual Covers")
    print("Q. Quit")
    print("-------------------------------------------")
    
    choice = input("Choose an option: ").strip().lower()
    
    if choice in ['q', 'quit']:
        sys.exit()
    
    fetch_mode = "None"
    force_rebuild = False
    only_manuals = False
    
    if choice == '1':
        fetch_mode = "OL"
        sub = input("\nHow do you want to run Open Library?\n1. Rebuild from ground up (Redownload All)\n2. Update (Only fetch missing)\nSelect option: ").strip()
        if sub == '1': force_rebuild = True
        
    elif choice == '2':
        fetch_mode = "GR"
        sub = input("\nHow do you want to run Goodreads?\n1. Rebuild from ground up (Redownload All)\n2. Update (Only fetch missing)\nSelect option: ").strip()
        if sub == '1': force_rebuild = True
        
    elif choice == '3':
        pass # Offline
        
    elif choice == '4':
        only_manuals = True
    
    else:
        print("Invalid option. Exiting.")
        sys.exit()

    ensure_directory(COVERS_DIR)
    ensure_directory(MANUAL_COVERS_DIR)
    
    # ... Reading Logic (Same) ...


    # 1. READ DATA
    books = []
    
    if os.path.exists(EXCEL_PATH):
        print(f"Reading {EXCEL_PATH}...")
        reader = SimpleXlsxReader(EXCEL_PATH)
        raw_rows = reader.read()
        
        for row in raw_rows:
            title = normalize_field(row.get('Title'))
            if not title: continue
            
            author = normalize_field(row.get('Author'))
            isbn = normalize_field(row.get('ISBN'))
            slug_source = isbn if isbn else f"{title}-{author}"
            
            books.append({
                'id': new_slug(slug_source),
                'title': title,
                'author': author,
                'isbn': isbn,
                'translator': normalize_field(row.get('Translator')),
                'publisher': normalize_field(row.get('Publisher')),
                'binding': normalize_field(row.get('Binding')),
                'published': parse_year(row.get('Published')),
                'cover': None,
                'spineColor': None,
                'spineTextColor': None
            })
            
    elif os.path.exists(BOOKS_JSON_PATH):
        print(f"No Excel file found. Using existing {BOOKS_JSON_PATH}...")
        try:
            with open(BOOKS_JSON_PATH, 'r', encoding='utf-8') as f:
                books = json.load(f)
        except Exception as e:
            print(f"Error reading JSON: {e}")
            sys.exit(1)
    else:
        print("Error: No input data found (Excel or JSON).")
        sys.exit(1)

    # 2. PROCESS
    print(f"Processing {len(books)} books...")
    
    stats = {'manuals': 0, 'downloads': 0, 'colored': 0, 'skipped': 0}
    has_imagemagick = shutil.which('convert') is not None
    if not has_imagemagick:
        print("\n[!] Warning: ImageMagick ('convert') not found.")
        print("    Color extraction will be skipped.")
        print("    To fix this, install ImageMagick:")
        print("      - Ubuntu/Debian: sudo apt install imagemagick")
        print("      - macOS: brew install imagemagick")
        
        install_choice = input("    Do you want to continue without color extraction? (Y/n): ").strip().lower()
        if install_choice == 'n':
            sys.exit()


    for book in books:
        isbn_clean = ""
        if book.get('isbn'):
            isbn_clean = re.sub(r'[^0-9Xx]', '', str(book['isbn'])).upper()
            
        filename = f"{isbn_clean}.jpg" if isbn_clean else f"{book['id']}.jpg"
        dest_path = os.path.join(COVERS_DIR, filename)
        
        # Check Manual
        manual_source = get_manual_cover_file(book, isbn_clean, MANUAL_COVERS_DIR)
        has_manual = False
        
        if manual_source:
            # We strictly want .jpg in the app
            if manual_source.lower().endswith(('.jpg', '.jpeg')) and shutil.copyfile(manual_source, dest_path):
                 has_manual = True
                 stats['manuals'] += 1
            elif has_imagemagick:
                # Use convert to make it jpg
                subprocess.run(['convert', manual_source, dest_path], check=False)
                if os.path.exists(dest_path):
                    has_manual = True
                    stats['manuals'] += 1

        # Download if needed
        if not has_manual and not only_manuals and fetch_mode != "None" and isbn_clean:
            
            # Skip if exists and not force rebuild
            if os.path.exists(dest_path) and not force_rebuild:
                pass
            else:
                dl_url = None
                print(f"  [{book['title']}]", end="", flush=True)

                if fetch_mode == "OL":
                    print(" Checking OpenLibrary...", end="", flush=True)
                    dl_url = OPEN_LIBRARY_URL.format(isbn=isbn_clean)
                
                elif fetch_mode == "GR":
                    print(" Fetching Goodreads...", end="", flush=True)
                    dl_url = get_goodreads_cover(isbn_clean)
                    if dl_url == "BLOCKED":
                        print(" !!! BLOCKED - Stopping Network !!!")
                        fetch_mode = "None"
                        dl_url = None

                if dl_url:
                    try:
                        req = urllib.request.Request(dl_url, headers={'User-Agent': 'BookshelfBuilder/1.0'})
                        with urllib.request.urlopen(req) as response:
                            data = response.read()
                            if len(data) > 1000:
                                with open(dest_path, 'wb') as f:
                                    f.write(data)
                                print(" Downloaded.")
                                stats['downloads'] += 1
                            else:
                                if os.path.exists(dest_path): os.remove(dest_path)
                                print(" Invalid/Empty.")
                    except Exception:
                        print(" Failed/Not Found.")
                else:
                    print(" No Cover Found.")
        
        # Color Extraction
        if os.path.exists(dest_path):
            # Update path in JSON (relative for web)
            book['cover'] = f"data/covers/{filename}"
            
            palette = extract_spine_colors_imagemagick(dest_path)
            if palette:
                book['spineColor'] = palette['background']
                book['spineTextColor'] = palette['text']
                stats['colored'] += 1
        else:
            book['cover'] = None
            book['spineColor'] = None
            book['spineTextColor'] = None
            stats['skipped'] += 1

    # 3. SAVE
    try:
        with open(BOOKS_JSON_PATH, 'w', encoding='utf-8') as f:
            json.dump(books, f, indent=4, ensure_ascii=False)
            
        with open(BOOKS_JS_PATH, 'w', encoding='utf-8') as f:
            f.write("window.booksData = ")
            json.dump(books, f, indent=4, ensure_ascii=False)
            f.write(";")
            
        print("------------------------------------------------")
        print("Done!")
        print(f"Books: {len(books)}")
        print(f"Manual Covers: {stats['manuals']}")
        print(f"Downloaded: {stats['downloads']}")
        print(f"Colors Extracted: {stats['colored']}")
        print("------------------------------------------------")
        
    except Exception as e:
        print(f"Error saving files: {e}")

    try:
        input("Press Enter to close the window")
    except:
        pass

if __name__ == "__main__":
    main()
