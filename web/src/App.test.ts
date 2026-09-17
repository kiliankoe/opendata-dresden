import { describe, expect, test } from "vitest";
import { formatHash, pageTitle, parseHash } from "./App";

describe("parseHash", () => {
  test("reads dataset, viewer and map mode", () => {
    expect(parseHash("#D/Karte/Kartendienst")).toEqual({
      openId: "D",
      viewer: "Karte",
      mapMode: "Kartendienst",
    });
  });

  test("leaves what a shorter link omits empty", () => {
    expect(parseHash("#D")).toEqual({
      openId: "D",
      viewer: "",
      mapMode: "",
    });
    expect(parseHash("")).toEqual({ openId: "", viewer: "", mapMode: "" });
  });

  // "Säulen-/Balkendiagramm" is a viewer label, so its slash must not read
  // as the separator
  test("keeps an encoded slash inside a label", () => {
    expect(
      parseHash(`#D/${encodeURIComponent("Säulen-/Balkendiagramm")}`),
    ).toEqual({
      openId: "D",
      viewer: "Säulen-/Balkendiagramm",
      mapMode: "",
    });
  });
});

describe("formatHash", () => {
  test("writes back what it read", () => {
    for (const hash of ["", "#D", "#D/Karte", "#D/Karte/Kartendienst"])
      expect(formatHash(parseHash(hash))).toBe(hash);
  });

  test("encodes labels", () => {
    expect(
      formatHash({
        openId: "D",
        viewer: "Säulen-/Balkendiagramm",
        mapMode: "",
      }),
    ).toBe(`#D/${encodeURIComponent("Säulen-/Balkendiagramm")}`);
  });

  test("drops a map mode that has no viewer to belong to", () => {
    expect(formatHash({ openId: "D", viewer: "", mapMode: "Objekte" })).toBe(
      "#D",
    );
  });
});

describe("pageTitle", () => {
  test("names the open dataset", () => {
    expect(pageTitle({ id: "D", title: "Baumkataster", resources: [] })).toBe(
      "OD3 - Baumkataster",
    );
  });

  test("falls back to the search page without one", () => {
    expect(pageTitle()).toBe("Dresden Open Data Suche");
  });
});
