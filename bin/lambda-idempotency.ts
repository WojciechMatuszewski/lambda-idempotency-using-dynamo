#!/usr/bin/env node
import "source-map-support/register";
import * as cdk from "aws-cdk-lib";
import { LambdaIdempotencyStack } from "../lib/lambda-idempotency-stack";

const app = new cdk.App();
new LambdaIdempotencyStack(app, "LambdaIdempotencyStack", {
  synthesizer: new cdk.DefaultStackSynthesizer({
    qualifier: "idemp"
  })
});
