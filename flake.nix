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
            pname = "dresden-opendata-mcp";
            version = builtins.head (
              builtins.match ".*const version = \"([^\"]+)\".*" (builtins.readFile ./cmd/dresden-opendata-mcp/main.go)
            );
            src = self;
            subPackages = [ "cmd/dresden-opendata-mcp" ];
            vendorHash = "sha256-nkijcLcSCEBOjEo+KipEv8Z9aAKnxT9IjavnnVIPJDI=";
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
