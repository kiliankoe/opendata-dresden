{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      forAllSystems = nixpkgs.lib.genAttrs nixpkgs.lib.systems.flakeExposed;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.buildGoModule {
            pname = "od3";
            version = builtins.head (
              builtins.match ".*const version = \"([^\"]+)\".*" (builtins.readFile ./cmd/od3/main.go)
            );
            src = self;
            subPackages = [ "cmd/od3" ];
            vendorHash = "sha256-OMDyDQu+pxxEJzyS/ybNFL5mv1Ir2047GMb2/6NxAO8=";
          };
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
            ];
          };
        }
      );
    };
}
