{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      forAllSystems = nixpkgs.lib.genAttrs nixpkgs.lib.systems.flakeExposed;

      od3 =
        { lib, buildGoModule }:
        buildGoModule {
          pname = "od3";
          version = builtins.head (
            builtins.match ".*const version = \"([^\"]+)\".*" (builtins.readFile ./cmd/od3/main.go)
          );
          src = self;
          subPackages = [ "cmd/od3" ];
          vendorHash = "sha256-OMDyDQu+pxxEJzyS/ybNFL5mv1Ir2047GMb2/6NxAO8=";
          meta = {
            description = "CLI and MCP server for Dresden's OpenData portal";
            homepage = "https://github.com/kiliankoe/opendata-dresden";
            license = lib.licenses.mit;
            mainProgram = "od3";
          };
        };
    in
    {
      # Adds `od3` to nixpkgs for use in NixOS or Home Manager configurations
      overlays.default = final: prev: { od3 = final.callPackage od3 { }; };

      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          od3 = pkgs.callPackage od3 { };
          default = self.packages.${system}.od3;
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              gopls
              gotools
              golangci-lint
              nodejs
              pnpm
            ];
          };
        }
      );
    };
}
