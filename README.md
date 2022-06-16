# Adding idempotency using Amazon DynamoDB

Inspired by [this entry](https://github.com/WojciechMatuszewski/programming-notes/blob/master/aws/serverless/learn-you-some-lambda-best-practice.md#idempotency) in my programming notes.

## Learnings

- One must add a sizable chunk of logic to implement this feature. It might be just me, but it would be neat to use a state machine for this use case.

- Locking is a must-have in a system that needs idempotency. Sending multiple consecutive requests is unlikely, but you will be grateful to have implemented the locking mechanism when it happens.

## Language

- I had a case where I wanted to wrap multiple errors into a single error.
  That seems to not be possible with the`fmt.Errorf`. You could use multiple `%w` tokens, but the `errors.Is` will not yield the correct (?) result if that is the case.

  ```go
    var SomeCustomErr = errors.New("Some custom error")

    func main() {
    newErr := errors.New("foo")
    wrapped := fmt.Errorf("failed %w, %w", newErr, SomeCustomErr)

        if errors.Is(wrapped, SomeCustomErr) {
            // Never called
            fmt.Println("Found!")
        }

        if errors.Is(wrapped, newErr) {
            // Never called
            fmt.Println("Found!")
        }

    }
  ```

  The solution is to use the error message from the first error and then use the `%w` token for the "SomeCustomError`.

- Using generics in the context of DynamoDB is quite fascinating. In Go, you have to provide the correct attribute type, but since the attribute value might have a generic type, you cannot know (unless you use reflection) what kind of attribute you should specify.

  - This is where the `MarshalMap` function comes in handy. It will do all the work for you – it uses reflection under the hood to apply the correct underlying structure.

- **Watch out when you create update expressions via the `dynamodbexpression` package**. The `WithUpdate` will **overwrite any previous call to the `WithUpdate`**.

  ```go
    expr, err := dynamodbexpression.
      NewBuilder().
      WithUpdate(
        dynamodbexpression.
          Set(dynamodbexpression.Name("result"), dynamodbexpression.Value(result)).
          Set(dynamodbexpression.Name("status"), dynamodbexpression.Value("COMPLETED")),
      ).
      Build()
  ```

  - If I were to split the two `Set` calls into separate `WithUpdate` calls, **only the last one would get applied**.

  - I've spent a lot of time trying to figure out what is going on – I had two properties to update, but only one was getting updated!

## Deployment

1. `npm run bootstrap`
2. `npm run deploy`
3. Send an POST HTTP request to the API. The `name` attribute in the request body is used as the idempotency key.
4. Send multiple requests via the `scratch.go` file.
