<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutApiAdminAuthConfigsByIdGrantedClient extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutBody $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutBody $requestBody = null)
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
        return str_replace(['{id}'], [$this->id], '/api/admin/auth-configs/{id}/granted-clients');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutBody) {
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
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdGrantedClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdGrantedClientNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse200', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdGrantedClientBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdGrantedClientNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}