<?php

namespace FlowCatalyst\Generated\Endpoint;

class ListApplicationUsers extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    /**
     * @param string $appCode Application code
     * @param array{
     *    "q"?: string, //Case-insensitive substring of name or email
     *    "active"?: string, //true | false; absent = both
     *    "clientId"?: string, //Only users homed at this client
     *    "roles"?: string, //CSV of the application's roles; users holding any
     *    "page"?: int, //0-based page (default 0)
     *    "pageSize"?: int, //Page size (default 50, max 200)
     *    "sortField"?: string, //name | email | createdAt (default createdAt)
     *    "sortOrder"?: string, //asc | desc (default asc)
     * } $queryParameters
     */
    public function __construct(string $appCode, array $queryParameters = [])
    {
        $this->appCode = $appCode;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}'], [$this->appCode], '/api/applications/{appCode}/users');
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
        $optionsResolver->setDefined(['q', 'active', 'clientId', 'roles', 'page', 'pageSize', 'sortField', 'sortOrder']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults([]);
        $optionsResolver->addAllowedTypes('q', ['string']);
        $optionsResolver->addAllowedTypes('active', ['string']);
        $optionsResolver->addAllowedTypes('clientId', ['string']);
        $optionsResolver->addAllowedTypes('roles', ['string']);
        $optionsResolver->addAllowedTypes('page', ['int']);
        $optionsResolver->addAllowedTypes('pageSize', ['int']);
        $optionsResolver->addAllowedTypes('sortField', ['string']);
        $optionsResolver->addAllowedTypes('sortOrder', ['string']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\ApplicationUserList|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApplicationUserList', 'json');
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