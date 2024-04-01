{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };
  outputs = { nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "nfpm";
          version = "unversioned";
          src = ./.;
          ldflags = [ "-s" "-w" "-X main.version=dev" "-X main.builtBy=flake" ];
          doCheck = false;
          vendorHash = "sha256-g57tLk2+WWcdG0COqkQD7eLYG0TdC0RnlhLF6Qt4woY=";
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            go-task
            gofumpt
            nix-prefetch
          ];
          shellHook = "go mod tidy";
        };

        # nix develop .#packagers
        devShells.packagers = pkgs.mkShell {
          packages = with pkgs; [
            apk-tools
            dpkg
            rpm
          ];
        };

        # nix develop .#docs
        devShells.docs = pkgs.mkShell {
          packages = with pkgs; with staging-pkgs.python311Packages; [
            (pkgs.writeScriptBin "ci-docs" "task docs:test")
            go-task
            htmltest
            mkdocs-material
            mkdocs-minify
          ] ++ mkdocs-material.passthru.optional-dependencies.git;
        };
      }
    );
}

