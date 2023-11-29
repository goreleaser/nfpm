{ pkgs ? import <nixpkgs> { } }:

pkgs.mkShell {
  packages = with pkgs; [
    go
    go-task
    gofumpt

    python311Packages.mkdocs-material
    python311Packages.mkdocs-minify
  ];
}
