<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutApiSdkPrincipalsByIdRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutBody $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutBody $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/sdk/principals/{id}/roles');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutBody) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkPrincipalsByIdRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkPrincipalsByIdRoleNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse200', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiSdkPrincipalsByIdRoleBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiSdkPrincipalsByIdRoleNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}