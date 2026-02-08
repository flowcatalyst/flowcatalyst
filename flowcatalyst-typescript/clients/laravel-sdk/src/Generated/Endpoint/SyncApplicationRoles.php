<?php

namespace FlowCatalyst\Generated\Endpoint;

class SyncApplicationRoles extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    /**
     * Bulk sync roles from an external application. Creates new roles, updates existing SDK roles. Set removeUnlisted=true to remove SDK roles not in the sync list.
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\SyncRolesRequest $requestBody
     * @param array{
     *    "removeUnlisted"?: bool,
     * } $queryParameters
     */
    public function __construct(string $appCode, ?\FlowCatalyst\Generated\Model\SyncRolesRequest $requestBody = null, array $queryParameters = [])
    {
        $this->appCode = $appCode;
        $this->body = $requestBody;
        $this->queryParameters = $queryParameters;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}'], [$this->appCode], '/api/applications/{appCode}/roles/sync');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\SyncRolesRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    protected function getQueryOptionsResolver(): \Symfony\Component\OptionsResolver\OptionsResolver
    {
        $optionsResolver = parent::getQueryOptionsResolver();
        $optionsResolver->setDefined(['removeUnlisted']);
        $optionsResolver->setRequired([]);
        $optionsResolver->setDefaults(['removeUnlisted' => false]);
        $optionsResolver->addAllowedTypes('removeUnlisted', ['bool']);
        return $optionsResolver;
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationRolesNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationRolesBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\SyncResponse2
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\SyncResponse2', 'json');
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SyncApplicationRolesNotFoundException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SyncApplicationRolesBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}