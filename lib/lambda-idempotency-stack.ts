import {
  aws_apigateway,
  aws_dynamodb,
  RemovalPolicy,
  Stack,
  StackProps
} from "aws-cdk-lib";
import { Construct } from "constructs";
import * as aws_lambda_go from "@aws-cdk/aws-lambda-go-alpha";
import { join } from "path";

export class LambdaIdempotencyStack extends Stack {
  constructor(scope: Construct, id: string, props?: StackProps) {
    super(scope, id, props);

    const dataTable = new aws_dynamodb.Table(this, "DataTable", {
      partitionKey: {
        name: "PK",
        type: aws_dynamodb.AttributeType.STRING
      },
      // sortKey: {
      //   name: "SK",
      //   type: aws_dynamodb.AttributeType.STRING
      // },
      billingMode: aws_dynamodb.BillingMode.PAY_PER_REQUEST,
      timeToLiveAttribute: "TTL",
      removalPolicy: RemovalPolicy.DESTROY
    });

    const handler = new aws_lambda_go.GoFunction(this, "Handler", {
      entry: join(__dirname, "../src/handler"),
      environment: {
        TABLE_NAME: dataTable.tableName
      }
    });
    dataTable.grantReadWriteData(handler);

    const api = new aws_apigateway.RestApi(this, "API", {
      defaultCorsPreflightOptions: {
        allowOrigins: aws_apigateway.Cors.ALL_ORIGINS,
        allowMethods: aws_apigateway.Cors.ALL_METHODS
      }
    });

    api.root.addMethod("POST", new aws_apigateway.LambdaIntegration(handler));
  }
}
