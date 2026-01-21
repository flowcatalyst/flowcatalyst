package tech.flowcatalyst.dispatchjob.model;

public enum DispatchProtocol {
    HTTP_WEBHOOK,
    GRPC,
    AWS_SQS,
    AWS_SNS,
    KAFKA,
    RABBITMQ
}
