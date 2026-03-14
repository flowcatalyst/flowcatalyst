<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiAdminRolesByApplicationByApplicationId extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $applicationId;
    /**
     * @param string $applicationId
     */
    public function __construct(string $applicationId)
    {
        $this->applicationId = $applicationId;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{applicationId}'], [$this->applicationId], '/api/admin/roles/by-application/{applicationId}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminRolesByApplicationApplicationIdGetResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminRolesByApplicationApplicationIdGetResponse200', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}