<?php

namespace FlowCatalyst\Generated\Endpoint;

class ListDispatchJobsRawAlias extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param array{
     *    "status"?: string,
     *    "clientId"?: string,
     *    "dispatchPoolId"?: string,
     *    "subscriptionId"?: string,
     *    "code"?: string,
     *    "since"?: string, //RFC3339 timestamp
     *    "until"?: string, //RFC3339 timestamp
     *    "messageGroup"?: string, //Exact message group
     *    "clientIds"?: string, //CSV of client ids
     *    "statuses"?: string, //CSV of statuses
     *    "applications"?: string, //CSV of application codes
     *    "subdomains"?: string, //CSV of subdomains
     *    "aggregates"?: string, //CSV of aggregates
     *    "codes"?: string, //CSV of codes
     *    "source"?: string, //Free-text source filter
     *    "eventType"?: string, //Exact event type. A job's code is its event type for a job raised by an event (and the caller's own job code for an SDK or function-dispatched job), so this filters the same column as code, by an index that also orders by createdAt. Combines with eventTypes and codes (all must hold).
     *    "eventTypes"?: string, //CSV of event types (at most 10; more is 400 TOO_MANY_EVENT_TYPES)
     *    "subscriptionIds"?: string, //CSV of subscription ids (at most 50; more is 400 TOO_MANY_SUBSCRIPTION_IDS). Combines with subscriptionId.
     *    "subscriptionCode"?: string, //Subscription code, exact; a trailing * makes it a prefix. Resolved to subscription ids first (400 TOO_MANY_MATCHES past 200); a code that matches nothing lists no jobs.
     *    "subscriptionName"?: string, //Subscription name, exact and case-insensitive; a trailing * makes it a prefix. Resolved like subscriptionCode.
     *    "connectionId"?: string, //Jobs delivered through this connection: the jobs of its subscriptions and the jobs that name the connection themselves (function dispatch).
     *    "connectionCode"?: string, //Connection code, exact; a trailing * makes it a prefix. Resolved to connection ids first (400 TOO_MANY_MATCHES past 50); a code that matches nothing lists no jobs.
     *    "limit"?: int,
     *    "offset"?: int,
     *    "size"?: int, //Max rows (default 50, max 1000)
     *    "sort"?: string, //createdAt.asc | createdAt.desc (default)
     * } $queryParameters
     */
    public function __construct(array $queryParameters = [])
    {
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return '/api/dispatch-jobs/raw';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['status', 'clientId', 'dispatchPoolId', 'subscriptionId', 'code', 'since', 'until', 'messageGroup', 'clientIds', 'statuses', 'applications', 'subdomains', 'aggregates', 'codes', 'source', 'eventType', 'eventTypes', 'subscriptionIds', 'subscriptionCode', 'subscriptionName', 'connectionId', 'connectionCode', 'limit', 'offset', 'size', 'sort']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('status', ['string']);
        $optionsResolver->addAllowedTypes('clientId', ['string']);
        $optionsResolver->addAllowedTypes('dispatchPoolId', ['string']);
        $optionsResolver->addAllowedTypes('subscriptionId', ['string']);
        $optionsResolver->addAllowedTypes('code', ['string']);
        $optionsResolver->addAllowedTypes('since', ['string']);
        $optionsResolver->addAllowedTypes('until', ['string']);
        $optionsResolver->addAllowedTypes('messageGroup', ['string']);
        $optionsResolver->addAllowedTypes('clientIds', ['string']);
        $optionsResolver->addAllowedTypes('statuses', ['string']);
        $optionsResolver->addAllowedTypes('applications', ['string']);
        $optionsResolver->addAllowedTypes('subdomains', ['string']);
        $optionsResolver->addAllowedTypes('aggregates', ['string']);
        $optionsResolver->addAllowedTypes('codes', ['string']);
        $optionsResolver->addAllowedTypes('source', ['string']);
        $optionsResolver->addAllowedTypes('eventType', ['string']);
        $optionsResolver->addAllowedTypes('eventTypes', ['string']);
        $optionsResolver->addAllowedTypes('subscriptionIds', ['string']);
        $optionsResolver->addAllowedTypes('subscriptionCode', ['string']);
        $optionsResolver->addAllowedTypes('subscriptionName', ['string']);
        $optionsResolver->addAllowedTypes('connectionId', ['string']);
        $optionsResolver->addAllowedTypes('connectionCode', ['string']);
        $optionsResolver->addAllowedTypes('limit', ['int']);
        $optionsResolver->addAllowedTypes('offset', ['int']);
        $optionsResolver->addAllowedTypes('size', ['int']);
        $optionsResolver->addAllowedTypes('sort', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\DispatchJobRead[]|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\DispatchJobRead[]', 'json');
        }
        if (mb_strpos(strtolower($contentType), 'application/json') !== false) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}