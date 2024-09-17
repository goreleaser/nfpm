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
          vendorHash = "";
        };

        devShells.default = pkgs.mkShell {
          shellHook = "go mod tidy";
        };

        # nix develop .#dev
        devShells.dev = pkgs.mkShell {
          packages = with pkgs; [
            go-task
            gofumpt
          ];
        };

        # nix develop .#packagers
        devShells.packagers = pkgs.mkShell {
          packages = with pkgs; [
            dpkg
          ] ++ (lib.optionals pkgs.stdenv.isLinux [
            apk-tools
            rpm
          ]);

        };

        # nix develop .#docs
        devShells.docs = pkgs.mkShell {
          packages = with pkgs; with pkgs.python311Packages; [
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

