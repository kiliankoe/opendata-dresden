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
            version = "0.1.0";
            src = self;
            subPackages = [ "cmd/server" ];
            vendorHash = "sha256-nkijcLcSCEBOjEo+KipEv8Z9aAKnxT9IjavnnVIPJDI=";
            postInstall = "mv $out/bin/server $out/bin/dresden-opendata-mcp";
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
