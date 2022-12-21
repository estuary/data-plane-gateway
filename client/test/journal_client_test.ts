import {
  assertEquals,
  assertMatch,
} from "https://deno.land/std@0.135.0/testing/asserts.ts";
// TODO: Built-in snapshot testing has landed in deno std, but is not yet released.
import { test as snapshotTest } from "https://deno.land/x/snap/mod.ts";

import * as broker from "../src/gen/broker/protocol/broker.ts";
import { BASE_URL, makeJwt } from "./test_support.ts";
import { JournalClient, parseJournalDocuments } from "../src/journal_client.ts";
import { JournalSelector } from "../src/selector.ts";
import { readStreamToEnd } from "../src/streams.ts";

Deno.test("JournalClient.list collection selector test", async () => {
  const client = new JournalClient(BASE_URL, await makeJwt({}));

  const name = new JournalSelector().collection("acmeCo/greetings");
  const journals = (await client.list(name)).unwrap();

  assertEquals(1, journals.length);
  assertEquals("acmeCo/greetings/pivot=00", journals[0].name);
});

Deno.test("JournalClient.list name selector test", async () => {
  const client = new JournalClient(BASE_URL, await makeJwt({}));

  const name = new JournalSelector().name("acmeCo/greetings/pivot=00");
  const journals = (await client.list(name)).unwrap();

  assertEquals(1, journals.length);
  assertEquals("acmeCo/greetings/pivot=00", journals[0].name);
});

Deno.test("JournalClient.list prefix selector test", async () => {
  const client = new JournalClient(BASE_URL, await makeJwt({}));
  const expectedJournals = [
    "ops/acmeCo/logs/kind=capture/name=acmeCo%2Fsource-hello-world/pivot=00",
    "ops/acmeCo/stats/kind=capture/name=acmeCo%2Fsource-hello-world/pivot=00",
  ];

  const prefixSelector = JournalSelector.prefix("ops/acmeCo/");
  const journals = (await client.list(prefixSelector)).unwrap();

  assertEquals(2, journals.length);
  assertEquals(expectedJournals, journals.map((j) => j.name).sort());
});

Deno.test("JournalClient.list exclusion selector test", async () => {
  const client = new JournalClient(BASE_URL, await makeJwt({}));

  const prefixSelector = JournalSelector.prefix("ops/acmeCo/");
  const excludedSelector = new JournalSelector().collection("ops/acmeCo/stats");
  const journals = (await client.list(prefixSelector, excludedSelector))
    .unwrap();

  assertEquals(1, journals.length);
  assertEquals(
    "ops/acmeCo/logs/kind=capture/name=acmeCo%2Fsource-hello-world/pivot=00",
    journals[0].name,
  );
});

Deno.test("JournalClient.list wrong signing key", async () => {
  const mismatchedKey = await makeJwt({
    key: new TextEncoder().encode("wrong key"),
  });
  const client = new JournalClient(BASE_URL, mismatchedKey);

  const name = new JournalSelector().collection("acmeCo/greetings");
  const error = (await client.list(name)).unwrap_err();

  assertMatch(error.body.message!, new RegExp("signature is invalid"));
});

Deno.test("JournalClient.list auth expiration", async () => {
  const expired_jwt = await makeJwt({ expiresAt: 1640995200 }); // Midnight Jan 1, 2022
  const client = new JournalClient(BASE_URL, expired_jwt);

  const name = new JournalSelector().collection("acmeCo/greetings");
  const error = (await client.list(name)).unwrap_err();

  assertMatch(error.body.message!, new RegExp("token is expired"));
});

Deno.test("JournalClient.list unauthorized prefix", async () => {
  const client = new JournalClient(BASE_URL, await makeJwt({}));

  const name = new JournalSelector()
    // An authorized selector...
    .collection("acmeCo/greetings")
    // ...as well as an unauthorized selector
    .collection("wayneEnterprises/batcave/sensors");
  const error = (await client.list(name)).unwrap_err();

  assertMatch(
    error.body.message!,
    new RegExp("unauthorized `estuary.dev/collection` label"),
  );
});

snapshotTest("JournalClient.read test", async ({ assertSnapshot }) => {
  const client = new JournalClient(BASE_URL, await makeJwt({}));

  const req = { journal: "acmeCo/greetings/pivot=00", endOffset: "1024" };
  const stream = (await client.read(req)).map_err(console.error).unwrap();
  const results: Array<broker.ProtocolReadResponse> = await readStreamToEnd(
    stream,
  );

  // Pluck out a few interesting properties to snapshot on. Many of the
  // properties are not stable between clusters, so snapshotting the entire
  // response would involve redacting a lot of fields.
  const pluck = (res: broker.ProtocolReadResponse) => {
    return {
      status: res.status,
      fragment: {
        journal: res!.fragment?.journal,
        compressionCodec: res!.fragment?.compressionCodec,
      },
    };
  };

  assertSnapshot(results.map(pluck));
});

snapshotTest("JournalClient.read content test", async ({ assertSnapshot }) => {
  const client = new JournalClient(BASE_URL, await makeJwt({}));

  const req = {
    journal: "acmeCo/greetings/pivot=00",
    offset: "10",
    endOffset: "1024",
  };
  const stream = (await client.read(req)).unwrap();
  const docStream = parseJournalDocuments(stream!);
  const results = await readStreamToEnd(docStream);

  let filtered_results = results.filter(r=>!(r._meta as any).ack)

  const masks = ["/*/_meta/uuid"];
  assertSnapshot(filtered_results, masks);
});

Deno.test("JournalClient.read unauthorized prefix", async () => {
  const client = new JournalClient(BASE_URL, await makeJwt({}));

  const req = {
    journal: "wayneEnterprises/batcave/sensors",
    endOffset: "1024",
  };
  const error = (await client.read(req)).unwrap_err();

  assertMatch(error.body.message!, new RegExp("was not found in claims"));
});
