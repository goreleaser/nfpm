"""Regenerate icon.dll: a resource-only PE with a single 1x1 blue icon.

Run from any directory with Python 3. No compiler or Windows SDK is required.
The fixture is never executed; Windows loads its RT_GROUP_ICON/RT_ICON data.
"""
from pathlib import Path
from struct import pack_into

image = bytearray(1024)
image[:2] = b"MZ"
pack_into("<I", image, 0x3C, 0x80)
image[0x80:0x84] = b"PE\0\0"
# IMAGE_FILE_HEADER: x86, one section, PE32 optional header, executable DLL.
pack_into("<HHIIIHH", image, 0x84, 0x14C, 1, 0, 0, 0, 224, 0x2102)
optional = 0x98
pack_into("<H", image, optional, 0x10B)
pack_into("<I", image, optional + 8, 512)  # SizeOfInitializedData
pack_into("<III", image, optional + 28, 0x10000000, 4096, 512)
pack_into("<HH", image, optional + 40, 6, 0)  # OS version
pack_into("<HH", image, optional + 48, 6, 0)  # subsystem version
pack_into("<II", image, optional + 56, 8192, 512)  # image and header sizes
pack_into("<H", image, optional + 68, 2)  # Windows GUI subsystem
pack_into("<IIIIII", image, optional + 72, 1048576, 4096, 1048576, 4096, 0, 16)
pack_into("<II", image, optional + 112, 0x1000, 228)  # resource data directory
section = optional + 224
pack_into("<8sIIIIIIHHI", image, section, b".rsrc", 228, 0x1000, 512, 512,
          0, 0, 0, 0, 0x40000040)

resource = 512

def directory(offset, entries):
    pack_into("<HH", image, resource + offset + 12, 0, len(entries))
    for index, (identifier, target) in enumerate(entries):
        pack_into("<II", image, resource + offset + 16 + 8 * index,
                  identifier, target)

# Resource types -> resource ID 1 -> US English -> data entry.
directory(0, [(3, 0x80000020), (14, 0x80000060)])
directory(32, [(1, 0x80000038)])
directory(56, [(1033, 80)])
pack_into("<IIII", image, resource + 80, 0x1000 + 160, 48, 0, 0)
directory(96, [(1, 0x80000078)])
directory(120, [(1033, 144)])
pack_into("<IIII", image, resource + 144, 0x1000 + 208, 20, 0, 0)
# BITMAPINFOHEADER, one BGRA pixel, and a padded transparent-mask row.
pack_into("<IiiHHIIiiII", image, resource + 160,
          40, 1, 2, 1, 32, 0, 4, 0, 0, 0, 0)
image[resource + 200:resource + 204] = bytes((255, 0, 0, 255))
# GRPICONDIR and GRPICONDIRENTRY referencing RT_ICON ID 1.
pack_into("<HHHBBBBHHIH", image, resource + 208,
          0, 1, 1, 1, 1, 0, 0, 1, 32, 48, 1)
Path(__file__).with_name("icon.dll").write_bytes(image)
