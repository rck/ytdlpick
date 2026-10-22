{
  description = "ytdlpick: interactive track picker for yt-dlp";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        default = ytdlpick;
        ytdlpick = pkgs.callPackage ./package.nix {
          version = self.shortRev or self.dirtyShortRev or "dev";
          src = self;
        };
      });

      overlays.default = final: prev: {
        ytdlpick = final.callPackage ./package.nix { src = self; };
      };

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          inputsFrom = [ self.packages.${pkgs.system}.ytdlpick ];
          packages = [
            pkgs.yt-dlp
            pkgs.gopls
          ];
        };
      });
    };
}
