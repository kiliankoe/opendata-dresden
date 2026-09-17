import { describe, expect, test } from "vitest";
import { formatHash, parseHash } from "./App";

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
