# Standalone FAT32 driver

This is a fork of github.com/diskfs/go-diskfs/filesystem/fat32@v1.8.0 with the
following changes to make it more suitable for embedded applications:

- Removed all dependencies to fmt to reduce binary footprint
- Simplified the backend interface to io.ReaderAt and optional io.WriterAt
- Reduced the memory footprint by accessing the file allocation table via a 4KB
  cache instead of reading it into a slice as a whole
