<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostApiSdkPrincipalsByIdClientByClientId extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    protected $clientId;
    /**
     * @param string $id
     * @param string $clientId
     */
    public function __construct(string $id, string $clientId)
    {
        $this->id = $id;
        $this->clientId = $clientId;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}', '{clientId}'], [$this->id, $this->clientId], '/api/sdk/principals/{id}/clients/{clientId}');
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
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsByIdClientByClientIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsByIdClientByClientIdConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse201
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse201', 'json');
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsByIdClientByClientIdNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse404', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsByIdClientByClientIdConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}