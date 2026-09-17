import { describe, expect, test } from "vitest";
import {
  byDate,
  createSearch,
  type Dataset,
  dateValue,
  mirrorPath,
  mirrorUrl,
  parseCsv,
  sortedResources,
} from "./datasets";

const dataset = (...resources: [string, string][]): Dataset => ({
  id: "D",
  title: "Test",
  resources: resources.map(([format, url]) => ({ format, url })),
});

const stats = dataset([
  "CSV",
  "https://opendata.dresden.de/dcat-ap/dataset/de-sn-dresden-einwohner_geborene_md121_2015/content.csv",
]);

describe("parseCsv", () => {
  test("splits the portal's semicolon separated rows", () => {
    expect(parseCsv("Jahr;Ort;Zahl\r\n2015;Altstadt;12\r\n")).toEqual([
      ["Jahr", "Ort", "Zahl"],
      ["2015", "Altstadt", "12"],
    ]);
  });

  // 16 of the mirrored tables quote a field precisely because it holds a
  // semicolon, so splitting on the separator alone loses columns
  test("keeps a semicolon inside a quoted field", () => {
    expect(parseCsv('a;"eins; zwei";c')).toEqual([["a", "eins; zwei", "c"]]);
  });

  test("unescapes doubled quotes", () => {
    expect(parseCsv('a;"sagt ""hallo""";c')).toEqual([
      ["a", 'sagt "hallo"', "c"],
    ]);
  });

  test("trims the padding the portal writes and skips blank lines", () => {
    expect(parseCsv("a ;  b\n\n c;d \n")).toEqual([
      ["a", "b"],
      ["c", "d"],
    ]);
  });

  test("reads plain newlines as well as the portal's CRLF", () => {
    expect(parseCsv("a;b\nc;d")).toEqual([
      ["a", "b"],
      ["c", "d"],
    ]);
  });
});

describe("mirrorPath", () => {
  test("names a statistics table without the shared prefix", () => {
    expect(mirrorPath(stats)).toBe(
      "data/statistics/einwohner_geborene_md121_2015.csv",
    );
    expect(mirrorUrl(stats)).toBe(
      "https://raw.githubusercontent.com/kiliankoe/opendata-dresden/main/data/statistics/einwohner_geborene_md121_2015.csv",
    );
  });

  test("ignores geodata layers, whose CSV is not mirrored", () => {
    const layer = dataset([
      "CSV",
      "https://kommisdd.dresden.de/net4/public/ogcapi/collections/L1527/items?format=csv/ewkt",
    ]);
    expect(mirrorPath(layer)).toBeUndefined();
    expect(mirrorUrl(layer)).toBeUndefined();
  });

  test("ignores datasets without a CSV at all", () => {
    expect(
      mirrorPath(dataset(["WMS", "https://example.org/wms"])),
    ).toBeUndefined();
  });
});

describe("dataset helpers", () => {
  test("sortedResources puts directly usable formats first", () => {
    const mixed = dataset(
      ["Information", "i"],
      ["WMS", "w"],
      ["GEOJSON", "g"],
      ["CSV", "c"],
    );
    expect(sortedResources(mixed).map((r) => r.format)).toEqual([
      "GEOJSON",
      "CSV",
      "WMS",
      "Information",
    ]);
  });

  test("dateValue reads the portal's dd.mm.yyyy, and nothing else", () => {
    expect(dateValue("15.07.2025")).toBe(Date.UTC(2025, 6, 15));
    expect(dateValue(undefined)).toBe(0);
  });

  test("byDate leads with the newest and keeps equal dates in order", () => {
    const d = (id: string, updated?: string) => ({ ...stats, id, updated });
    const order = byDate(
      [
        d("A", "01.01.2020"),
        d("B"),
        d("C", "02.01.2020"),
        d("D", "01.01.2020"),
      ],
      "updated",
    );
    expect(order.map((entry) => entry.id)).toEqual(["C", "A", "D", "B"]);
  });

  // Most datasets have no change date yet. Falling back to the portal's date
  // keeps that list readable instead of dropping it into index order.
  test("byDate trails datasets without a change date, in update order", () => {
    const d = (id: string, changed: string | undefined, updated: string) => ({
      ...stats,
      id,
      changed,
      updated,
    });
    const order = byDate(
      [
        d("A", undefined, "03.01.2020"),
        d("B", "01.01.2020", "01.01.2020"),
        d("C", undefined, "04.01.2020"),
        d("D", "02.01.2020", "01.01.2020"),
      ],
      "changed",
    );
    expect(order.map((entry) => entry.id)).toEqual(["D", "B", "C", "A"]);
  });

  test("search folds umlauts both ways", () => {
    const datasets: Dataset[] = [
      { ...stats, id: "A", title: "Straßenbahn" },
      { ...stats, id: "B", title: "Bevölkerung" },
    ];
    const search = createSearch(datasets);
    expect(search.search("strasse").map((r) => r.id)).toEqual(["A"]);
    expect(search.search("bevoelkerung").map((r) => r.id)).toEqual(["B"]);
  });
});
