# Statistics tables

One CSV per statistics dataset of the OpenData portal, copied byte for byte and
refreshed nightly by [`index.yml`](../../.github/workflows/index.yml). Their git
history is the point of keeping them here: it records how the published numbers
change, which the portal itself does not.

The file name is the dataset's DCAT identifier without the shared
`de-sn-dresden-` prefix. [`data/index.json`](../index.json) maps it back to the
dataset's title, source and portal links.

Geodata layers are not mirrored. Their attribute tables alone come to roughly
1.3 GB per snapshot against 39 MB for these, see issue #4.

## Attribution

Source: Landeshauptstadt Dresden, <https://opendata.dresden.de>

License: [Datenlizenz Deutschland Namensnennung 2.0](https://www.govdata.de/dl-de/by-2-0)

The files are unmodified copies of the portal's own downloads.
