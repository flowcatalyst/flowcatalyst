<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiAdminClientsByIdApplicationsByAppIdDisable extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    protected $appId;
    /**
     * @param string $id
     * @param string $appId
     */
    public function __construct(string $id, string $appId)
    {
        $this->id = $id;
        $this->appId = $appId;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}', '{appId}'], [$this->id, $this->appId], '/api/admin/clients/{id}/applications/{appId}/disable');
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdApplicationsByAppIdDisableNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse200', 'json');
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdApplicationsByAppIdDisableNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}